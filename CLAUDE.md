# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WatchCat — 自动化游戏服务健康检查系统。通过可配置的多步骤计划（Plan），定时执行 HTTP/WebSocket 测试，记录结果，暴露 Prometheus 指标。

## Build & Run

```bash
# 编译
go build -o watchcat ./cmd/server/main.go

# 运行（需要 PostgreSQL，配置见 config.yaml）
./watchcat

# 测试
go test ./...
go test ./internal/engine -v -run TestRenderNestedJSON  # 单个测试

# Docker 开发栈（Prometheus + Grafana）
docker-compose up -d
```

服务默认监听 `0.0.0.0:3003`，指标端点 `/metrics`。

## Architecture

```
cmd/server/main.go          # 入口：初始化DB、模板、路由、调度器、优雅关闭
internal/
  config/config.go          # YAML配置加载（环境变量 > config.yaml > 默认值）
  model/                    # GORM 模型：Plan, Step, ExecLog + AutoMigrate
  store/                    # 数据访问层：PlanStore, StepStore, LogStore
  handler/                  # Echo HTTP 处理器，每个 handler 自注册路由
  engine/
    runner.go               # 核心：按序执行步骤，per-plan 互斥锁防并发
    scheduler.go            # Cron 调度器，定时触发 plan 执行并写日志
    context.go              # 变量系统：{{var}} 模板渲染，gjson 嵌套路径
    executor_http.go        # HTTP 步骤执行
    executor_ws.go          # WebSocket 步骤执行
    executor_delay.go       # 延迟步骤执行
    executor_common.go      # 捕获规则 + 断言规则（通用逻辑）
  metrics/prometheus.go     # Prometheus 指标定义和记录
web/
  templates/                # Go html/template + HTMX
    layouts/base.html       # 基础布局
    partials/               # 可复用片段（plan_row, log_detail 等）
    plans/ steps/ logs/     # 页面模板
  static/vendor/            # 前端库（HTMX 等）
```

## Key Design Patterns

- **模板系统**：4种渲染模式 — 完整页面(base layout)、内容片段(content block)、独立片段(file basename)、命名片段(partial block)。HTMX 请求通过 `helpers.go` 的 `IsHTMX()` 判断后返回片段而非整页。
- **变量传递**：Plan 的 `variables` (JSONB) 初始化上下文，每步的 `captures` 从响应提取值，通过 `CtxMap` 跨步骤传递。模板语法 `{{varName}}` 支持 gjson 嵌套路径。
- **断言系统**：`AssertionRule` 支持 `eq/ne/gt/lt/contains` 操作符，可检查 `status/body/header`。
- **SSE 实时流**：`/plans/:id/exec/stream` 端点逐步推送执行结果。

## Configuration

`config.yaml` 为主配置文件，支持环境变量覆盖：`DATABASE_URI`、`SERVER_PORT`、`SERVER_HOST`、`LOG_LEVEL`。`.env` 文件通过 godotenv 加载。

## Data Model

- **Plan**: 检测计划（name, cron, variables, enabled）
- **Step**: 执行步骤（plan_id, seq, type[http/ws/delay], config, captures, assertions）
- **ExecLog**: 执行日志（plan_id, status[success/failed/timeout], duration_ms, step_results）

所有 ID 使用 UUID，JSON 字段存储为 PostgreSQL JSONB。

## Tech Stack

Go 1.24 · Echo v4 · GORM + PostgreSQL · robfig/cron v3 · Prometheus · gjson · gorilla/websocket · HTMX

## Conventions

- 全站中文化（UI文本、错误消息、日志输出）
- Handler 通过 `RegisterRoutes(e)` 自注册路由
- 模块名 `watchcat`（go.mod）
