# MiMoCode2API

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-12MB-2496ED?logo=docker)](https://www.docker.com)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

> 设备指纹 → JWT → OpenAI 兼容 API。零成本白嫖 MiMo 免费模型。

## 这是什么

从 [MiMo-Code](https://github.com/XiaomiMiMo/MiMo-Code) 源码逆向分析，发现其内置免费通道：`api.xiaomimimo.com`。无需注册，无需 API Key，只需设备指纹交换 JWT 即可调用。

本项目是一个 **12MB 的 Go 反向代理**，自动完成指纹生成 → JWT 引导 → 请求透传，对外暴露标准 OpenAI 兼容接口。

```
curl/ Hermes/ Cursor
    │  POST /v1/chat/completions
    │  {model: "mimo/mimo-auto", messages: [...], stream: true}
    ▼
┌──────────────────────────────────────────┐
│  mimo2api (Go, 单进程, 12MB Docker)      │
│                                          │
│  ① SHA256(hostname|os|arch|cpu|user)     │
│  ② POST /api/free-ai/bootstrap → JWT     │
│  ③ 透传 + JWT header + model 重写        │
│  ④ 非流式: 剥 SSE "data:" 前缀           │
└──────────────────────────────────────────┘
    │  POST /api/free-ai/openai/chat
    │  Authorization: Bearer <jwt>
    │  X-Mimo-Source: mimocode-cli-free
    ▼
  api.xiaomimimo.com  (已 OpenAI 兼容)
```

## 快速开始

```bash
# 无需任何配置，直接启动
docker compose up -d --build
```

10 秒后代理就绪：

```bash
# 健康检查
curl http://localhost:10000/health

# 模型列表
curl http://localhost:10000/v1/models

# 流式对话
curl -N http://localhost:10000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"mimo/mimo-auto","messages":[{"role":"user","content":"Hello"}],"stream":true}'

# 非流式对话
curl http://localhost:10000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"mimo/mimo-auto","messages":[{"role":"user","content":"Hello"}],"stream":false}'
```

## 接入 Hermes Agent

```yaml
# ~/.hermes/config.yaml
custom_providers:
  - name: mimocode
    base_url: http://127.0.0.1:10000/v1
    api_key: ""
    model: mimo/mimo-auto

model:
  default: mimo/mimo-auto
  provider: custom:mimocode
```

```bash
hermes chat -q "Hello"  # 直接使用
```

## 模型信息

| 属性 | 值 |
|------|-----|
| ID | `mimo/mimo-auto` |
| 上下文窗口 | 1,000,000 tokens |
| 最大输出 | 128,000 tokens |
| 输入模态 | 文本 + 图片 |
| 推理 | ✅ 支持 |
| 流式 | ✅ SSE |
| 非流式 | ✅ JSON |
| 工具调用 | 由 Hermes system prompt 管理 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `API_KEY` | (空) | 代理认证，空=无认证 |
| `MIMO_FREE_BASE_URL` | `https://api.xiaomimimo.com` | 上游 |
| `MIMO_FINGERPRINT` | (自动生成) | 设备指纹 |
| `MIMO2API_DEBUG` | `false` | 调试日志 |
| `MIMO2API_PORT` | `10000` | 监听端口 |

## 原理

### 逆向分析

从 `MiMo-Code/packages/opencode/src/plugin/mimo-free.ts` 提取：

```typescript
// 1. 设备指纹
const seed = [hostname, platform, arch, cpu, username].join("|")
const fingerprint = crypto.createHash("sha256").update(seed).digest("hex")

// 2. 引导获取 JWT
POST /api/free-ai/bootstrap  {"client": fingerprint}  →  {"jwt": "..."}

// 3. 调用 API
POST /api/free-ai/openai/chat
Authorization: Bearer <jwt>
X-Mimo-Source: mimocode-cli-free
```

### 为什么只需透传

上游 API 已经是 OpenAI 兼容格式，不需要任何消息格式转换。代理只做：

1. **指纹生成** — SHA256 哈希
2. **JWT 管理** — 自动引导 + 过期前 5 分钟刷新 + 401 自动重试
3. **Model 重写** — `mimo/mimo-auto` → `mimo-auto`（上游不支持 provider 前缀）
4. **非流式修复** — 上游返回 `data:{json}`，代理剥除 `data:` 前缀

### 工具调用

MiMo 模型不输出标准 OpenAI `tool_calls` 格式。工具调用由 Hermes Agent 的 system prompt 管理，代理层不做任何 tool prompt 注入。

## 从源码构建

```bash
git clone https://github.com/Sliverkiss/mimocode2api.git
cd mimocode2api
go build -o mimo2api .
MIMO2API_PORT=10000 ./mimo2api
```

## 项目结构

```
mimocode2api/
├── main.go                   # 入口
├── go.mod
├── Dockerfile                # 多阶段构建
├── docker-compose.yml
├── .env.example
├── internal/
│   ├── config/config.go      # 配置加载
│   ├── proxy/proxy.go        # 指纹 + JWT + 反向代理
│   ├── handler/handler.go    # HTTP 处理
│   ├── middleware/auth.go    # 可选认证
│   └── model/schema.go       # 模型列表
└── analysis/                 # 逆向分析文档
    ├── 01-architecture.md    # MiMo-Code 架构
    ├── 02-api-protocol.md    # HTTP API 协议
    ├── 03-tool-calling.md    # 工具调用机制
    └── 04-free-api.md        # 免费通道逆向
```

## 免责声明

本项目仅供学习和研究使用。请遵守 MiMo 服务条款。

## License

MIT