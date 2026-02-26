package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"watchcat/internal/config"
	"watchcat/internal/engine"
	"watchcat/internal/handler"
	"watchcat/internal/metrics"
	authmw "watchcat/internal/middleware"
	"watchcat/internal/model"
	"watchcat/internal/store"
)

// templateEntry holds a pre-compiled template and which block to execute.
type templateEntry struct {
	tmpl  *template.Template
	block string
}

// TemplateRenderer implements echo.Renderer with per-page template instances
// to avoid {{define "content"}} conflicts across pages.
type TemplateRenderer struct {
	entries map[string]templateEntry
}

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	entry, ok := t.entries[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return entry.tmpl.ExecuteTemplate(w, entry.block, data)
}

// newTemplateRenderer builds the template registry.
//
// Template types:
//  1. Full pages: base layout + partials + page content → execute "base.html"
//  2. Content partials: partials + page content → execute "content" block
//  3. Fragment templates: partials + standalone fragment → execute file basename
//  4. Named partials: partials only → execute specific named block
func newTemplateRenderer(baseDir string) *TemplateRenderer {
	funcMap := template.FuncMap{
		"dict": func(values ...any) map[string]any {
			d := make(map[string]any, len(values)/2)
			for i := 0; i < len(values)-1; i += 2 {
				d[values[i].(string)] = values[i+1]
			}
			return d
		},
		"safeJS": func(v any) template.JS {
			switch s := v.(type) {
			case string:
				return template.JS(s)
			case []byte:
				return template.JS(s)
			default:
				return template.JS(fmt.Sprintf("%v", v))
			}
		},
	}

	r := &TemplateRenderer{entries: make(map[string]templateEntry)}

	layoutFile := filepath.Join(baseDir, "layouts/base.html")
	partialFiles, _ := filepath.Glob(filepath.Join(baseDir, "partials/*.html"))

	// parseWithLayout: base layout + partials + page file
	parseWithLayout := func(pageFile string) *template.Template {
		files := make([]string, 0, len(partialFiles)+2)
		files = append(files, layoutFile)
		files = append(files, partialFiles...)
		files = append(files, filepath.Join(baseDir, pageFile))
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
	}

	// parseContent: partials + page file (no layout, for HTMX content partials)
	parseContent := func(pageFile string) *template.Template {
		files := make([]string, 0, len(partialFiles)+1)
		files = append(files, partialFiles...)
		files = append(files, filepath.Join(baseDir, pageFile))
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
	}

	// parsePartialsOnly: just the partials (for named block rendering)
	parsePartialsOnly := func() *template.Template {
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(partialFiles...))
	}

	// parseFragment: partials + standalone fragment file (no {{define}} wrapper)
	parseFragment := func(fragFile string) *template.Template {
		files := make([]string, 0, len(partialFiles)+1)
		files = append(files, partialFiles...)
		files = append(files, filepath.Join(baseDir, fragFile))
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
	}

	// --- Full pages (with base layout, execute "base.html") ---
	fullPages := map[string]string{
		"plans/list.html":    "plans/list.html",
		"plans/create.html":  "plans/form.html",
		"plans/edit.html":    "plans/form.html",
		"plans/detail.html":  "plans/detail.html",
		"dashboard":          "dashboard.html",
		"alerts":             "alerts.html",
		"scripts/list.html":  "scripts/list.html",
		"scripts/create.html": "scripts/form.html",
		"scripts/edit.html":  "scripts/form.html",
		"scripts/detail.html": "scripts/detail.html",
	}
	for name, file := range fullPages {
		r.entries[name] = templateEntry{tmpl: parseWithLayout(file), block: "base.html"}
	}

	// --- Content partials (no layout, execute "content" block) ---
	contentPartials := map[string]string{
		"plans/_list.html":    "plans/list.html",
		"plans/_detail.html":  "plans/detail.html",
		"scripts/_list.html":  "scripts/list.html",
		"scripts/_detail.html": "scripts/detail.html",
	}
	for name, file := range contentPartials {
		r.entries[name] = templateEntry{tmpl: parseContent(file), block: "content"}
	}

	// --- Fragment templates (no {{define}}, execute file basename) ---
	fragments := map[string]string{
		"steps/_list.html":             "steps/list.html",
		"steps/_edit.html":             "steps/form.html",
		"logs/_list.html":              "logs/list.html",
		"scripts/_plan_scripts.html":   "scripts/plan_scripts.html",
		"scripts/_step_scripts.html":   "scripts/step_scripts.html",
	}
	for name, file := range fragments {
		r.entries[name] = templateEntry{
			tmpl:  parseFragment(file),
			block: filepath.Base(file),
		}
	}

	// --- Named partials (execute specific named block from partials) ---
	partialsOnlyTmpl := parsePartialsOnly()
	r.entries["plans/_row.html"] = templateEntry{tmpl: partialsOnlyTmpl, block: "plan_row"}
	r.entries["logs/_detail.html"] = templateEntry{tmpl: partialsOnlyTmpl, block: "log_detail"}
	r.entries["dashboard/_uptime"] = templateEntry{tmpl: partialsOnlyTmpl, block: "uptime_section"}

	// --- Dashboard section partials (defined in dashboard.html, for HTMX endpoints) ---
	dashContent := parseContent("dashboard.html")
	r.entries["dashboard/_stats"] = templateEntry{tmpl: dashContent, block: "dashboard_stats"}
	r.entries["dashboard/_failures"] = templateEntry{tmpl: dashContent, block: "dashboard_failures"}
	r.entries["dashboard/_trend"] = templateEntry{tmpl: dashContent, block: "dashboard_trend"}

	// --- Login page (standalone, no layout) ---
	loginTmpl := template.Must(template.New("login.html").Funcs(funcMap).ParseFiles(
		filepath.Join(baseDir, "login.html"),
	))
	r.entries["login"] = templateEntry{tmpl: loginTmpl, block: "login.html"}

	return r
}

func main() {
	// 1. Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 2. Initialize DB
	db, err := gorm.Open(postgres.Open(cfg.Database.URI), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)

	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 3. Initialize stores
	planStore := store.NewPlanStore(db)
	stepStore := store.NewStepStore(db)
	logStore := store.NewLogStore(db)
	scriptStore := store.NewScriptStore(db)

	// 4. Initialize metrics
	m := metrics.NewMetrics("1.0.0", "dev", "go1.24")

	// 5. Initialize runner + scheduler
	runner := engine.NewRunner()
	runner.SetMetrics(m)

	scriptRunner := engine.NewScriptRunner(scriptStore)
	runner.SetScriptRunner(scriptRunner)

	scheduler := engine.NewScheduler(runner, planStore, logStore)
	scheduler.SetMetrics(m)

	// 6. Initialize Echo
	e := echo.New()
	e.HideBanner = true

	// Custom error handler: log the actual error
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		msg := err.Error()
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			msg = fmt.Sprintf("%v", he.Message)
		}
		log.Printf("[error] %s %s → %d: %s", c.Request().Method, c.Request().URL.Path, code, msg)
		if !c.Response().Committed {
			c.JSON(code, map[string]string{"message": msg})
		}
	}

	// Template renderer
	e.Renderer = newTemplateRenderer("web/templates")

	// Static files
	e.Static("/static", "web/static")

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("[http] %s %s → %d", v.Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	// Auth middleware
	if cfg.Auth.Enabled {
		e.Use(authmw.AuthRequired(cfg.Auth))
	}

	// 7. Auth routes (public, before other routes)
	authHandler := handler.NewAuthHandler(cfg.Auth)
	authHandler.RegisterRoutes(e)

	// 8. Register routes
	planHandler := handler.NewPlanHandler(planStore, stepStore, logStore, scheduler)
	planHandler.RegisterRoutes(e)

	stepHandler := handler.NewStepHandler(stepStore)
	stepHandler.RegisterRoutes(e)

	execHandler := handler.NewExecHandler(planStore, logStore, runner)
	execHandler.RegisterRoutes(e)

	logHandler := handler.NewLogHandler(logStore)
	logHandler.RegisterRoutes(e)

	dashHandler := handler.NewDashboardHandler(logStore, planStore)
	dashHandler.Register(e)

	alertHandler := handler.NewAlertHandler(logStore, planStore)
	alertHandler.Register(e)

	scriptHandler := handler.NewScriptHandler(scriptStore)
	scriptHandler.RegisterRoutes(e)

	// 8. /metrics endpoint
	e.GET(cfg.Metrics.Path, echo.WrapHandler(
		promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}),
	))

	// 9. Start scheduler
	if cfg.Scheduler.Enabled {
		if err := scheduler.Start(); err != nil {
			log.Printf("警告: 调度器启动失败: %v", err)
		}
	}

	// 10. Log retention cleanup goroutine
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			deleted, err := logStore.CleanupOlderThan(cfg.LogRetention.Days)
			if err != nil {
				log.Printf("日志清理失败: %v", err)
			} else if deleted > 0 {
				log.Printf("已清理 %d 条过期日志", deleted)
			}
		}
	}()

	// 11. Graceful shutdown
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器错误: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务...")
	scheduler.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Printf("Echo 关闭错误: %v", err)
	}

	sqlDB.Close()
	log.Println("服务器已停止")
}
