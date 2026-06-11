# MiMo-Code Architecture Analysis

## 概览

MiMo-Code 是小米开源的 TUI AI 编程助手，基于 TypeScript/Bun 构建，使用 Vercel AI SDK 作为 LLM 抽象层。

## 仓库结构 (Monorepo)

```
MiMo-Code/
├── packages/
│   ├── opencode/       # 核心 AI 引擎 (CLI + Server)
│   ├── app/            # TUI 前端 (SolidJS)
│   ├── console/        # 后端管理系统
│   │   ├── core/       # 数据库/模型管理/认证
│   │   ├── function/   # 云函数 (认证/日志)
│   │   └── app/        # 管理后台 UI
│   ├── desktop/        # Electron 桌面应用
│   ├── enterprise/     # 企业版 Web 前端
│   ├── sdk/js/         # JavaScript SDK (@opencode-ai/sdk)
│   ├── ui/             # 共享 UI 组件库
│   ├── plugin/         # 插件系统
│   ├── shared/         # 共享工具库
│   └── function/       # 后端云函数
├── sdks/vscode/        # VS Code 扩展
└── infra/              # SST 基础设施
```

## 核心包: packages/opencode

### 技术栈

- **运行时**: Bun (Node.js 兼容)
- **语言**: TypeScript
- **Web 框架**: Hono (HTTP 服务器)
- **LLM 抽象**: Vercel AI SDK (`ai` 包)
- **状态管理**: Effect (函数式效果系统)
- **数据库**: SQLite (Drizzle ORM)
- **TUI**: SolidJS + @opentui/core

### 关键模块

```
src/
├── provider/          # LLM 提供者抽象层
│   ├── provider.ts    # 核心提供者管理 (支持 20+ 提供者)
│   ├── models.ts      # 模型列表 (从 models.dev 获取)
│   ├── schema.ts      # ProviderID/ModelID 类型
│   └── sdk/copilot/   # OpenAI 兼容协议的 Copilot 适配器
├── session/           # 会话管理
│   ├── prompt.ts      # 提示词处理引擎 (3355行, 核心)
│   ├── system.ts      # 系统提示词生成
│   ├── message.ts     # 消息管理
│   ├── compaction.ts  # 上下文压缩
│   └── processor.ts   # 消息处理器
├── server/            # HTTP API 服务器
│   ├── server.ts      # 服务器入口
│   ├── middleware.ts   # 认证/CORS/压缩中间件
│   └── routes/instance/
│       ├── session.ts  # 会话 API (CRUD + prompt)
│       ├── provider.ts # 提供者配置 API
│       └── question.ts # 问题/权限 API
├── tool/              # 工具系统 (bash, edit, read, grep, websearch...)
├── agent/             # Agent 配置和 prompt
├── mcp/               # MCP 协议集成
├── config/            # 配置管理
└── auth/              # 认证管理
```

## 提供者架构

MiMo-Code 支持 20+ LLM 提供者，均通过 Vercel AI SDK 统一接口：

```typescript
// provider.ts — 提供者注册
const BUNDLED_PROVIDERS = {
  "@ai-sdk/openai": () => import("@ai-sdk/openai").then(m => m.createOpenAI),
  "@ai-sdk/anthropic": () => import("@ai-sdk/anthropic").then(m => m.createAnthropic),
  "@ai-sdk/google": () => import("@ai-sdk/google").then(m => m.createGoogleGenerativeAI),
  "@ai-sdk/openai-compatible": () => import("@ai-sdk/openai-compatible").then(m => m.createOpenAICompatible),
  "@ai-sdk/github-copilot": () => import("./sdk/copilot").then(m => m.createOpenaiCompatible),
  // ... 20+ providers
}
```

### 模型路由 (Zen 系统)

`packages/console/core/src/model.ts` 管理 "Zen" 模型系统：

- 每个 MiMo 模型 (如 `mimo-auto`) 配置多个上游提供者
- 提供者有优先级、权重、TPM 限制
- 支持故障转移 (fallbackProvider)
- 支持试用提供者 (trialProvider)

## 认证架构

### CLI 端认证
- `auth.json` 存储提供者 API Key
- 支持 OAuth (GitHub/Google) 和 API Key 两种模式
- `MIMOCODE_AUTH_CONTENT` 环境变量可注入认证内容

### 服务端认证
- `MIMOCODE_SERVER_PASSWORD` 设置服务端密码
- HTTP Basic Auth: `Authorization: Basic base64(mimocode:password)`
- 客户端通过 `x-mimocode-directory` header 指定工作目录

### Console 端认证
- 支持邮箱、GitHub、Google 三种登录方式
- API Key 管理 (KeyTable)
- 工作空间隔离

## 数据流

```
用户输入 (TUI/VS Code)
    │
    ▼
mimo CLI (index.ts)
    │
    ▼
SessionPrompt (prompt.ts)
    │
    ├── SystemPrompt 生成
    ├── Message 历史管理
    ├── Tool 注册
    │
    ▼
LLM 调用 (session/llm.ts)
    │
    ▼
Provider 层 (provider.ts)
    │
    ├── 选择提供者 (根据 providerID)
    ├── 加载 SDK (动态 import)
    ├── 调用 AI SDK
    │
    ▼
SSE 流式响应
    │
    ├── reasoning-delta → 展示思考
    ├── text-delta → 展示回复
    ├── tool-input-start/delta/end → 工具调用
    └── finish → 完成
```

## 关键发现

1. **Vercel AI SDK 是核心抽象层** — 所有 LLM 调用都通过它
2. **模型通过 `models.dev` API 动态获取** — 缓存 5 分钟，每小时刷新
3. **Zen 模型系统是多层路由** — `mimo-auto` 不是单一模型，而是路由到多个上游
4. **工具调用被 AI SDK 标准化** — 不同提供者的 tool calling 格式被统一
5. **服务端通过 Hono 暴露 HTTP API** — 与 TUI 通过相同协议通信