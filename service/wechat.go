package service

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"server/global"
	"server/model/response"
	"strings"
	"sync"
	"time"
)

const (
	wechatTokenURL  = "https://api.weixin.qq.com/cgi-bin/token"
	wechatTicketURL = "https://api.weixin.qq.com/cgi-bin/ticket/getticket"
)

type WechatService struct {
	mu          sync.Mutex
	accessToken cachedWechatValue
	jsapiTicket cachedWechatValue
}

type cachedWechatValue struct {
	value     string
	expiresAt time.Time
}

type wechatTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type wechatTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
}

func (wechatService *WechatService) JSConfig(ctx context.Context, pageURL string) (response.WechatJSConfig, error) {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		return response.WechatJSConfig{}, errors.New("url is required")
	}

	appID, appSecret := wechatCredentials()
	if appID == "" || appSecret == "" {
		return response.WechatJSConfig{}, errors.New("wechat app_id or app_secret is empty")
	}

	ticket, err := wechatService.jsapiTicketValue(ctx, appID, appSecret)
	if err != nil {
		return response.WechatJSConfig{}, err
	}

	nonceStr, err := randomNonce()
	if err != nil {
		return response.WechatJSConfig{}, err
	}
	timestamp := time.Now().Unix()
	signatureRaw := fmt.Sprintf(
		"jsapi_ticket=%s&noncestr=%s&timestamp=%d&url=%s",
		ticket,
		nonceStr,
		timestamp,
		pageURL,
	)
	sum := sha1.Sum([]byte(signatureRaw))

	return response.WechatJSConfig{
		AppID:     appID,
		Timestamp: timestamp,
		NonceStr:  nonceStr,
		Signature: hex.EncodeToString(sum[:]),
	}, nil
}

func (wechatService *WechatService) jsapiTicketValue(ctx context.Context, appID, appSecret string) (string, error) {
	wechatService.mu.Lock()
	if wechatService.jsapiTicket.value != "" && time.Now().Before(wechatService.jsapiTicket.expiresAt) {
		value := wechatService.jsapiTicket.value
		wechatService.mu.Unlock()
		return value, nil
	}
	wechatService.mu.Unlock()

	accessToken, err := wechatService.accessTokenValue(ctx, appID, appSecret)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("access_token", accessToken)
	params.Set("type", "jsapi")

	var parsed wechatTicketResponse
	if err := getWechatJSON(ctx, wechatTicketURL+"?"+params.Encode(), &parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("wechat jsapi ticket failed: %s", parsed.ErrMsg)
	}
	if parsed.Ticket == "" {
		return "", errors.New("wechat jsapi ticket is empty")
	}

	wechatService.mu.Lock()
	wechatService.jsapiTicket = cachedWechatValue{
		value:     parsed.Ticket,
		expiresAt: wechatExpiresAt(parsed.ExpiresIn),
	}
	wechatService.mu.Unlock()
	return parsed.Ticket, nil
}

func (wechatService *WechatService) accessTokenValue(ctx context.Context, appID, appSecret string) (string, error) {
	wechatService.mu.Lock()
	if wechatService.accessToken.value != "" && time.Now().Before(wechatService.accessToken.expiresAt) {
		value := wechatService.accessToken.value
		wechatService.mu.Unlock()
		return value, nil
	}
	wechatService.mu.Unlock()

	params := url.Values{}
	params.Set("grant_type", "client_credential")
	params.Set("appid", appID)
	params.Set("secret", appSecret)

	var parsed wechatTokenResponse
	if err := getWechatJSON(ctx, wechatTokenURL+"?"+params.Encode(), &parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("wechat access token failed: %s", parsed.ErrMsg)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("wechat access token is empty")
	}

	wechatService.mu.Lock()
	wechatService.accessToken = cachedWechatValue{
		value:     parsed.AccessToken,
		expiresAt: wechatExpiresAt(parsed.ExpiresIn),
	}
	wechatService.mu.Unlock()
	return parsed.AccessToken, nil
}

func wechatCredentials() (string, string) {
	appID := strings.TrimSpace(os.Getenv("WECHAT_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("WECHAT_APP_SECRET"))
	if appID == "" {
		appID = strings.TrimSpace(global.Config.Wechat.AppID)
	}
	if appSecret == "" {
		appSecret = strings.TrimSpace(global.Config.Wechat.AppSecret)
	}
	return appID, appSecret
}

func getWechatJSON(ctx context.Context, requestURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("wechat request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func randomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func wechatExpiresAt(expiresIn int) time.Time {
	if expiresIn <= 600 {
		expiresIn = 600
	}
	return time.Now().Add(time.Duration(expiresIn-300) * time.Second)
}
