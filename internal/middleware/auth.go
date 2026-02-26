package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"watchcat/internal/config"
)

const cookieName = "watchcat_session"

// AuthRequired 返回一个 Echo 中间件，校验签名 Cookie。
// 未认证时重定向到 /login。
func AuthRequired(cfg config.AuthConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !cfg.Enabled {
				return next(c)
			}

			// 跳过公开路径
			path := c.Request().URL.Path
			if path == "/login" || strings.HasPrefix(path, "/static/") || path == "/metrics" {
				return next(c)
			}

			cookie, err := c.Cookie(cookieName)
			if err != nil || !ValidateSession(cookie.Value, cfg.Secret, cfg.MaxAge) {
				if c.Request().Header.Get("HX-Request") == "true" {
					c.Response().Header().Set("HX-Redirect", "/login")
					return c.NoContent(http.StatusUnauthorized)
				}
				return c.Redirect(http.StatusFound, "/login")
			}

			return next(c)
		}
	}
}

// GenerateSession 生成签名的 session 值: "username|timestamp|signature"
func GenerateSession(username, secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := sign(username+"|"+ts, secret)
	return username + "|" + ts + "|" + sig
}

// SetSessionCookie 设置认证 Cookie。
func SetSessionCookie(c echo.Context, username, secret string, maxAge int) {
	value := GenerateSession(username, secret)
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie 清除认证 Cookie。
func ClearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ValidateSession 校验 session 值的签名和过期时间。
func ValidateSession(value, secret string, maxAge int) bool {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 {
		return false
	}

	username, tsStr, sig := parts[0], parts[1], parts[2]

	// 校验签名
	expected := sign(username+"|"+tsStr, secret)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return false
	}

	// 校验过期
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix()-ts > int64(maxAge) {
		return false
	}

	return true
}

// sign 使用 HMAC-SHA256 签名。
func sign(data, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprint(h, data)
	return hex.EncodeToString(h.Sum(nil))
}
