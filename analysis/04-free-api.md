# MiMo Free API 逆向分析

## 关键发现

从 `packages/opencode/src/plugin/mimo-free.ts` 提取：

## 免费通道

MiMo-Code 内置免费通道，无需注册、无需 API Key。

### API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `https://api.xiaomimimo.com/api/free-ai/bootstrap` | POST | 设备指纹 → JWT |
| `https://api.xiaomimimo.com/api/free-ai/openai/chat` | POST | OpenAI 兼容聊天 |
| `https://api.xiaomimimo.com/api/free-ai/openai/models` | GET | 模型列表 |

### 认证流程

```
1. 生成设备指纹
   SHA256(hostname|platform|arch|cpu_model|username)

2. 引导获取 JWT
   POST /api/free-ai/bootstrap
   Body: {"client": "<sha256-fingerprint>"}
   Response: {"jwt": "<jwt-token>"}

3. 所有 API 请求附 JWT
   Authorization: Bearer <jwt>
   X-Mimo-Source: mimocode-cli-free
```

### JWT 管理

- JWT 缓存，过期前 5 分钟自动刷新
- 401/403 响应触发重新引导
- JWT 的 `exp` 从 payload 解码

### 设备指纹

```typescript
const seed = [os.hostname(), process.platform, process.arch, cpu, username].join("|")
const fingerprint = crypto.createHash("sha256").update(seed).digest("hex")
```

Go 实现:

```go
seed := fmt.Sprintf("%s|%s|%s|%s|%s", hostname, runtime.GOOS, runtime.GOARCH, cpu, username)
hash := sha256.Sum256([]byte(seed))
fingerprint := fmt.Sprintf("%x", hash)
```

### 请求头

```
Authorization: Bearer <jwt>
X-Mimo-Source: mimocode-cli-free
Content-Type: application/json
User-Agent: (随机轮转)
```

### 模型配置

```json
{
  "mimo-auto": {
    "name": "MiMo Auto",
    "attachment": true,
    "reasoning": true,
    "tool_call": true,
    "temperature": true,
    "modalities": {
      "input": ["text", "image"],
      "output": ["text"]
    },
    "limit": {
      "context": 1000000,
      "output": 128000
    },
    "cost": {
      "input": 0,
      "output": 0
    }
  }
}
```

### URL 重写

上游聊天端点使用 `/chat` 而非 `/chat/completions`:

```typescript
const rewritten = url.replace(/\/chat\/completions(\?|$)/, "/chat$1")
```

### 提供者配置

```typescript
{
  provider: {
    mimo: {
      name: "MiMo Auto (free)",
      npm: "@ai-sdk/openai-compatible",
      api: "https://api.xiaomimimo.com/api/free-ai/openai",
      options: {
        apiKey: "anonymous",
        fetch: wrappedFetch,  // 自动注入 JWT + 401 重试
      },
      models: { "mimo-auto": { ... } }
    }
  }
}
```

## 代理设计

代理只需要做三件事：

1. **生成设备指纹** — SHA256 哈希
2. **引导 JWT** — POST bootstrap 端点
3. **透传请求** — 添加 JWT auth header，转发到上游

上游已经是 OpenAI 兼容格式，不需要任何转换。

## 与之前方案的对比

| 维度 | 旧方案 (CLI 包装) | 新方案 (JWT 代理) |
|------|------------------|-------------------|
| 进程数 | 5 个 mimo 进程 | 1 个 Go 进程 |
| 认证 | 需要 MiMo 账号 | 免费，无需注册 |
| 镜像大小 | 300MB+ (含 Node.js) | ~12MB (纯 Go) |
| 启动时间 | 2 分钟+ | 毫秒级 |
| 复杂度 | 消息格式转换 | 纯透传 |
| 可靠性 | 依赖 CLI 稳定性 | 只依赖 HTTP |