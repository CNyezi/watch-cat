<p align="center">
  <img src="web/static/logo.svg" alt="WatchCat Logo" width="128" />
</p>

<h1 align="center">WatchCat</h1>

<p align="center">
  自动化服务健康检查系统 —— 可配置的多步骤计划执行引擎
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white" alt="Go 1.24" />
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="Apache 2.0 License" />
  <img src="https://img.shields.io/docker/image-size/yezi/watchcat/latest?logo=docker&label=image" alt="Docker Image" />
</p>

---

WatchCat 是一个轻量级的服务健康检查平台。通过可视化界面配置多步骤检测计划（Plan），支持 HTTP / WebSocket / 延迟 等步骤类型，Cron 定时调度执行，实时查看执行结果，并暴露 Prometheus 指标供 Grafana 监控告警。

## 功能特性

- **多步骤计划** — 将多个请求编排为一个检测流程，步骤间可传递变量
- **多协议支持** — HTTP（全方法）、WebSocket（含 Pomelo 协议）、延迟步骤
- **变量系统** — 计划级变量 + 步骤间捕获传递，支持 `{{var}}` 模板语法和 gjson 嵌套路径
- **断言引擎** — 对 status / body / header 做 eq / ne / gt / lt / contains 断言
- **JavaScript 脚本** — Pre-request / Post-response 脚本，支持请求签名、动态 Header 注入（goja 引擎）
- **Cron 调度** — 基于 robfig/cron 的定时执行，支持秒级精度
- **SSE 实时流** — 点击"立即执行"可实时观看每步进度
- **Prometheus 指标** — 计划成功率、执行耗时、HTTP 状态码分布等全维度指标
- **Grafana 面板** — 内置预配置 Dashboard，开箱即用
- **暗色主题 UI** — 基于 HTMX + Alpine.js + Tailwind CSS 的现代化 Web 界面
- **Cookie 认证** — HMAC-SHA256 签名的 Session 认证，保护全站路由


## 快速开始

### 环境要求

- Go 1.24+
- PostgreSQL 15+

### 本地运行

```bash
# 克隆仓库
git clone https://github.com/yezi/watchcat.git
cd watchcat

# 配置数据库（编辑 config.yaml 或设置环境变量）
export DATABASE_URI="postgres://user:pass@localhost:5432/watchcat?sslmode=disable"

# 编译运行
go build -o watchcat ./cmd/server/main.go
./watchcat
```

服务启动后访问 `http://localhost:3003`。

### Docker 部署

```bash
docker run -d \
  --name watchcat \
  -p 3003:3003 \
  -e DATABASE_URI="postgres://user:pass@db-host:5432/watchcat?sslmode=disable" \
  yezi/watchcat:latest
```

### Docker Compose（含 Prometheus + Grafana）

```bash
# 启动完整监控栈
docker-compose up -d

# 服务地址：
# WatchCat:   http://localhost:3003
# Prometheus: http://localhost:9090
# Grafana:    http://localhost:3001 (admin/admin)
```

## 配置

主配置文件 `config.yaml`，所有选项均支持环境变量覆盖：

| 配置项 | 环境变量 | 默认值 | 说明 |
|--------|---------|--------|------|
| `server.port` | `SERVER_PORT` | `3003` | 服务端口 |
| `server.host` | `SERVER_HOST` | `0.0.0.0` | 监听地址 |
| `database.uri` | `DATABASE_URI` | — | PostgreSQL 连接串（必填） |
| `log.level` | `LOG_LEVEL` | `info` | 日志级别 |
| `metrics.enabled` | — | `true` | 启用 Prometheus 指标 |
| `metrics.path` | — | `/metrics` | 指标端点路径 |
| `scheduler.enabled` | — | `true` | 启用 Cron 调度器 |
| `log_retention.days` | — | `30` | 执行日志保留天数 |
| `auth.enabled` | — | `true` | 启用登录认证 |
| `auth.username` | — | `admin` | 登录用户名 |
| `auth.password` | — | — | 登录密码 |
| `auth.secret` | — | — | Cookie 签名密钥 |
| `auth.max_age` | — | `604800` | Session 有效期（秒，默认 7 天） |

## 核心概念

### Plan（检测计划）

一个 Plan 定义了一组按序执行的检测步骤。每个 Plan 可以配置：

- **Cron 表达式** — 定时调度规则
- **超时时间** — 整个计划的最大执行时长
- **变量** — 初始上下文变量（JSON 格式），在步骤间共享

### Step（执行步骤）

每个步骤属于一个 Plan，按序号执行。支持三种类型：

| 类型 | 说明 | 典型用途 |
|------|------|---------|
| **HTTP** | 发送 HTTP 请求 | API 健康检查、接口测试 |
| **WebSocket** | WebSocket 连接与消息收发（支持 Pomelo 协议） | 长连接服务测试 |
| **Delay** | 等待指定毫秒数 | 步骤间添加延迟 |

每个步骤可以配置 **捕获规则**（从响应中提取变量传递给后续步骤）和 **断言规则**（校验响应是否符合预期）。

### Script（JavaScript 脚本）

脚本在步骤执行前后运行，用于动态修改请求或处理响应：

```javascript
// Pre-request: 生成签名
const timestamp = Date.now().toString();
const sign = crypto.hmacSHA256(ctx.vars.secret, timestamp);
ctx.request.headers["X-Timestamp"] = timestamp;
ctx.request.headers["X-Sign"] = sign;
```

脚本可以绑定到 Plan（对所有步骤生效）或单个 Step。

## 项目结构

```
├── cmd/server/main.go           # 入口
├── internal/
│   ├── config/                  # 配置加载
│   ├── model/                   # 数据模型（Plan, Step, ExecLog, Script）
│   ├── store/                   # 数据访问层
│   ├── handler/                 # HTTP 处理器
│   ├── engine/
│   │   ├── runner.go            # 计划执行引擎
│   │   ├── scheduler.go         # Cron 调度器
│   │   ├── context.go           # 变量系统
│   │   ├── executor_http.go     # HTTP 步骤执行器
│   │   ├── executor_ws.go       # WebSocket 步骤执行器
│   │   ├── executor_delay.go    # 延迟步骤执行器
│   │   ├── script_engine.go     # JavaScript 脚本引擎（goja）
│   │   └── pomelo/              # Pomelo 协议实现
│   ├── metrics/                 # Prometheus 指标
│   └── middleware/              # 认证中间件
├── web/
│   ├── templates/               # Go html/template + HTMX
│   └── static/                  # 前端静态资源
├── data/
│   ├── prometheus/              # Prometheus 配置 + 告警规则
│   └── grafana/                 # Grafana Dashboard 预配置
├── config.yaml                  # 默认配置
├── Dockerfile                   # 多阶段构建
└── docker-compose.yaml          # 完整监控栈
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go 1.24 · Echo v4 · GORM |
| 数据库 | PostgreSQL（JSONB） |
| 脚本引擎 | goja（Go 原生 JS 引擎） |
| 调度 | robfig/cron v3 |
| 监控 | Prometheus · Grafana |
| 前端 | HTMX · Alpine.js · Tailwind CSS · CodeMirror 6 |
| WebSocket | gorilla/websocket · Pomelo 协议 |
| JSON 解析 | gjson（嵌套路径提取） |

## Prometheus 指标

WatchCat 暴露以下指标（默认 `/metrics`）：

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `watchcat_plan_up` | Gauge | 计划健康状态（1=正常, 0=异常） |
| `watchcat_plan_executions_total` | Counter | 计划执行总次数（按结果分） |
| `watchcat_plan_duration_seconds` | Histogram | 计划执行耗时分布 |
| `watchcat_plans_running` | Gauge | 当前正在执行的计划数 |
| `watchcat_plans_registered` | Gauge | 已注册计划总数 |
| `watchcat_plans_enabled` | Gauge | 已启用计划数 |
| `watchcat_step_duration_seconds` | Histogram | 步骤执行耗时分布 |
| `watchcat_step_http_responses_total` | Counter | HTTP 步骤响应码分布 |
| `watchcat_step_ws_up` | Gauge | WebSocket 连接状态 |

## License

[Apache License 2.0](LICENSE)
