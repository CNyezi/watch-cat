package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"watchcat/internal/config"
	authmw "watchcat/internal/middleware"
)

type AuthHandler struct {
	cfg config.AuthConfig
}

func NewAuthHandler(cfg config.AuthConfig) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// RegisterRoutes 注册登录/登出路由（这些路由不需要认证）。
func (h *AuthHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/login", h.LoginPage)
	e.POST("/login", h.Login)
	e.POST("/logout", h.Logout)
}

// LoginPage 渲染登录页面。
func (h *AuthHandler) LoginPage(c echo.Context) error {
	// 已登录则跳转首页
	if cookie, err := c.Cookie("watchcat_session"); err == nil {
		if authmw.ValidateSession(cookie.Value, h.cfg.Secret, h.cfg.MaxAge) {
			return c.Redirect(http.StatusFound, "/")
		}
	}
	return c.Render(http.StatusOK, "login", nil)
}

// Login 处理登录表单提交。
func (h *AuthHandler) Login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username != h.cfg.Username || password != h.cfg.Password {
		return c.Render(http.StatusOK, "login", map[string]interface{}{
			"Error": "用户名或密码错误",
		})
	}

	authmw.SetSessionCookie(c, username, h.cfg.Secret, h.cfg.MaxAge)
	return c.Redirect(http.StatusFound, "/")
}

// Logout 清除 Cookie 并重定向到登录页。
func (h *AuthHandler) Logout(c echo.Context) error {
	authmw.ClearSessionCookie(c)
	return c.Redirect(http.StatusFound, "/login")
}
