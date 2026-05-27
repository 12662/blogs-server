package api

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"server/global"
	"server/model/elasticsearch"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	shareMetaStart = "<!-- share-meta:start -->"
	shareMetaEnd   = "<!-- share-meta:end -->"
)

type sharePageMeta struct {
	Title       string
	Description string
	Keywords    string
	Image       string
	URL         string
	Type        string
	SiteName    string
}

func (articleApi *ArticleApi) HomeSharePage(c *gin.Context) {
	origin := requestOrigin(c)
	siteTitle := fallbackString(global.Config.Website.Title, "赵的个人博客")
	description := fallbackString(global.Config.Website.Description, "个人博客记录生活，分享技术文章、新闻热点与日常灵感。")
	image := absolutePublicURL(c, fallbackString(global.Config.Website.Logo, "/image/webLogo.jpg"))

	renderSharePage(c, sharePageMeta{
		Title:       siteTitle,
		Description: description,
		Keywords:    strings.Join([]string{siteTitle, "个人博客", "技术博客", "Vue", "Go"}, ","),
		Image:       image,
		URL:         origin + "/",
		Type:        "website",
		SiteName:    siteTitle,
	})
}

func (articleApi *ArticleApi) ArticleSharePage(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.Status(http.StatusNotFound)
		return
	}

	article, err := articleService.Get(id)
	if err != nil {
		global.Log.Error("failed to build article share page", zap.String("article_id", id), zap.Error(err))
		c.Status(http.StatusNotFound)
		return
	}

	renderSharePage(c, articleShareMeta(c, id, article))
}

func articleShareMeta(c *gin.Context, id string, article elasticsearch.Article) sharePageMeta {
	siteTitle := fallbackString(global.Config.Website.Title, "赵的个人博客")
	title := fallbackString(article.Title, siteTitle)
	description := fallbackString(article.Abstract, fallbackString(article.Keyword, title))
	keywords := article.Keyword
	if keywords == "" && len(article.Tags) > 0 {
		keywords = strings.Join(article.Tags, ",")
	}

	return sharePageMeta{
		Title:       title,
		Description: description,
		Keywords:    keywords,
		Image:       absolutePublicURL(c, fallbackString(article.Cover, fallbackString(global.Config.Website.Logo, "/image/webLogo.jpg"))),
		URL:         requestOrigin(c) + "/article/" + url.PathEscape(id),
		Type:        "article",
		SiteName:    siteTitle,
	}
}

func renderSharePage(c *gin.Context, meta sharePageMeta) {
	indexHTML, err := loadIndexHTML()
	if err != nil {
		global.Log.Error("failed to load index html for share page", zap.Error(err))
		c.String(http.StatusInternalServerError, "index.html not found")
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(injectShareMeta(indexHTML, meta)))
}

func loadIndexHTML() (string, error) {
	cwd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(cwd, "../web/dist/index.html"),
		filepath.Join(cwd, "web/dist/index.html"),
		filepath.Join(cwd, "dist/index.html"),
		filepath.Join(cwd, "../web/index.html"),
		filepath.Join(cwd, "web/index.html"),
	}

	var lastErr error
	for _, candidate := range candidates {
		data, err := os.ReadFile(filepath.Clean(candidate))
		if err == nil {
			return string(data), nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no index.html candidates configured")
	}
	return "", lastErr
}

func injectShareMeta(indexHTML string, meta sharePageMeta) string {
	metaHTML := buildShareMetaHTML(meta)
	start := strings.Index(indexHTML, shareMetaStart)
	end := strings.Index(indexHTML, shareMetaEnd)
	if start >= 0 && end > start {
		end += len(shareMetaEnd)
		return indexHTML[:start] + shareMetaStart + "\n" + metaHTML + "\n    " + shareMetaEnd + indexHTML[end:]
	}

	return strings.Replace(indexHTML, "</head>", metaHTML+"\n  </head>", 1)
}

func buildShareMetaHTML(meta sharePageMeta) string {
	title := escapeMeta(fallbackString(meta.Title, "赵的个人博客"))
	description := escapeMeta(fallbackString(meta.Description, title))
	keywords := escapeMeta(meta.Keywords)
	image := escapeMeta(meta.Image)
	pageURL := escapeMeta(meta.URL)
	pageType := escapeMeta(fallbackString(meta.Type, "website"))
	siteName := escapeMeta(fallbackString(meta.SiteName, "赵的个人博客"))

	lines := []string{
		"    <title>" + title + "</title>",
		`    <meta name="description" content="` + description + `">`,
	}
	if keywords != "" {
		lines = append(lines, `    <meta name="keywords" content="`+keywords+`">`)
	}
	lines = append(lines,
		`    <link rel="canonical" href="`+pageURL+`">`,
		`    <meta property="og:type" content="`+pageType+`">`,
		`    <meta property="og:site_name" content="`+siteName+`">`,
		`    <meta property="og:title" content="`+title+`">`,
		`    <meta property="og:description" content="`+description+`">`,
		`    <meta property="og:url" content="`+pageURL+`">`,
		`    <meta property="og:image" content="`+image+`">`,
		`    <meta property="og:image:secure_url" content="`+image+`">`,
		`    <meta property="og:image:alt" content="`+title+`">`,
		`    <meta property="og:image:width" content="300">`,
		`    <meta property="og:image:height" content="300">`,
		`    <meta name="twitter:card" content="summary">`,
		`    <meta name="twitter:title" content="`+title+`">`,
		`    <meta name="twitter:description" content="`+description+`">`,
		`    <meta name="twitter:image" content="`+image+`">`,
		`    <meta itemprop="name" content="`+title+`">`,
		`    <meta itemprop="description" content="`+description+`">`,
		`    <meta itemprop="image" content="`+image+`">`,
	)
	return strings.Join(lines, "\n")
}

func requestOrigin(c *gin.Context) string {
	proto := firstHeaderValue(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := firstHeaderValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	return proto + "://" + host
}

func absolutePublicURL(c *gin.Context, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "/image/webLogo.jpg"
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "//") {
		proto := firstHeaderValue(c.GetHeader("X-Forwarded-Proto"))
		if proto == "" {
			proto = "https"
		}
		return proto + ":" + value
	}
	if strings.HasPrefix(value, "/") {
		return requestOrigin(c) + value
	}
	return requestOrigin(c) + "/" + value
}

func fallbackString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func escapeMeta(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
}

func firstHeaderValue(value string) string {
	if comma := strings.Index(value, ","); comma >= 0 {
		value = value[:comma]
	}
	return strings.TrimSpace(value)
}
