package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"server/global"
	"server/model/other"
	"server/utils"
)

// GaodeService 提供高德 IP 定位和天气查询能力。
type GaodeService struct {
}

// GetLocationByIP 根据 IP 地址获取地理位置信息。
func (gaodeService *GaodeService) GetLocationByIP(ip string) (other.IPResponse, error) {
	data := other.IPResponse{}
	key := global.Config.Gaode.Key
	urlStr := "https://restapi.amap.com/v3/ip"
	method := "GET"
	params := map[string]string{
		"ip":  ip,
		"key": key,
	}
	res, err := utils.HttpRequest(urlStr, method, nil, params, nil)
	if err != nil {
		return data, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return data, fmt.Errorf("request failed with status code: %d", res.StatusCode)
	}

	byteData, err := io.ReadAll(res.Body)
	if err != nil {
		return data, err
	}

	// 高德在定位失败时可能把 province/city 返回成 []，不能直接反序列化到 string。
	var raw struct {
		Status    string          `json:"status"`
		Info      string          `json:"info"`
		InfoCode  string          `json:"infocode"`
		Province  json.RawMessage `json:"province"`
		City      json.RawMessage `json:"city"`
		Adcode    string          `json:"adcode"`
		Rectangle string          `json:"rectangle"`
	}
	if err := json.Unmarshal(byteData, &raw); err != nil {
		return data, err
	}
	if raw.Status != "1" {
		return data, fmt.Errorf("gaode ip api failed: %s(%s)", raw.Info, raw.InfoCode)
	}

	data.Status = raw.Status
	data.Info = raw.Info
	data.InfoCode = raw.InfoCode
	data.Province = flexibleString(raw.Province)
	data.City = flexibleString(raw.City)
	data.Adcode = raw.Adcode
	data.Rectangle = raw.Rectangle
	return data, nil
}

// GetWeatherByAdcode 根据城市编码获取实时天气信息。
func (gaodeService *GaodeService) GetWeatherByAdcode(adcode string) (other.Live, error) {
	data := other.WeatherResponse{}
	key := global.Config.Gaode.Key
	urlStr := "https://restapi.amap.com/v3/weather/weatherInfo"
	method := "GET"
	params := map[string]string{
		"city":       adcode,
		"key":        key,
		"extensions": "base",
	}
	res, err := utils.HttpRequest(urlStr, method, nil, params, nil)
	if err != nil {
		return other.Live{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return other.Live{}, fmt.Errorf("request failed with status code: %d", res.StatusCode)
	}

	byteData, err := io.ReadAll(res.Body)
	if err != nil {
		return other.Live{}, err
	}

	if err := json.Unmarshal(byteData, &data); err != nil {
		return other.Live{}, err
	}
	if data.Status != "1" {
		return other.Live{}, fmt.Errorf("gaode weather api failed: %s(%s)", data.Info, data.InfoCode)
	}
	if len(data.Lives) == 0 {
		return other.Live{}, fmt.Errorf("no live weather data available")
	}
	return data.Lives[0], nil
}

func flexibleString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil && len(values) > 0 {
		return values[0]
	}
	return ""
}
