# MiMo-Code HTTP API 协议逆向

## 概述

`mimo serve` 命令启动一个 Hono HTTP 服务器，暴露 REST API 供 TUI 和 SDK 客户端使用。

## 启动方式

```bash
# 设置密码 (可选，不设置则无认证)
export MIMOCODE_SERVER_PASSWORD="your-password"

# 启动服务器
mimo serve --port 10001 --hostname 127.0.0.1
```

## 认证

所有 API 请求需要 HTTP Basic Auth：

```
Authorization: Basic base64("mimocode:<password>")
```

如果未设置 `MIMOCODE_SERVER_PASSWORD`，服务器无认证保护。

## API 端点

### 1. 会话管理

#### POST /session — 创建会话

```json
// 请求
{
  "title": "optional-title",
  "permission": [],  // 权限规则
  "parentID": "optional-parent-session-id"
}

// 响应
{
  "id": "session_01J...",
  "title": "optional-title",
  "directory": "/path/to/project",
  "parentID": null,
  "time": {
    "created": 1700000000000,
    "updated": 1700000000000
  }
}
```

#### GET /session — 列出会话

查询参数: `directory` (过滤项目目录), `roots` (只返回根会话), `start` (时间戳), `search` (标题搜索), `limit` (数量限制)

#### GET /session/:id — 获取会话详情

#### DELETE /session/:id — 删除会话

#### PATCH /session/:id — 更新会话

```json
{ "title": "new-title", "permission": [...], "time": { "archived": 1700000000000 } }
```

#### POST /session/:id/fork — 分叉会话

#### POST /session/:id/abort — 中止运行中的会话

### 2. 消息管理

#### POST /session/:id/prompt — 发送提示词 (核心端点)

这是最重要的端点，用于发送用户消息并获取 AI 流式响应。

```json
// 请求体
{
  "modelID": "mimo-v2-5-pro",
  "providerID": "mimo",
  "messageID": "msg_01J...",  // 前端生成的 ULID
  "parts": [
    {
      "type": "text",
      "text": "用户消息内容"
    }
  ],
  "system": "可选的系统提示词覆盖",
  "tools": {
    "bash": true,
    "read": true,
    "edit": true,
    "grep": true,
    "glob": true,
    "write": true,
    "task": true,
    "websearch": true,
    "webfetch": true,
    "question": true
  }
}
```

**响应**: SSE 流，Content-Type: `application/json` (行分隔 JSON，每行一个事件)

**SSE 事件类型**:

#### 事件: message.updated

```json
{
  "type": "message.updated",
  "properties": {
    "info": {
      "id": "msg_01J...",
      "sessionID": "session_01J...",
      "role": "assistant",
      "parentID": "msg-parent_01J...",
      "modelID": "mimo-v2-5-pro",
      "providerID": "mimo",
      "mode": "default",
      "path": {
        "cwd": "/path/to/project",
        "root": "/path/to/project"
      },
      "cost": 0.00123,
      "tokens": {
        "input": 500,
        "output": 200,
        "reasoning": 100,
        "cache": { "read": 0, "write": 0 }
      },
      "finish": "stop",
      "error": null,
      "time": {
        "created": 1700000000000,
        "completed": 1700000005000
      }
    }
  }
}
```

#### 事件: message.part.updated (核心流式事件)

```json
{
  "type": "message.part.updated",
  "properties": {
    "part": {
      "id": "part_01J...",
      "sessionID": "session_01J...",
      "messageID": "msg_01J...",
      "type": "text",         // part 类型 (见下方)
      "text": "AI 回复内容",   // 完整文本 (非流式)
      "time": { "start": 1700000000000, "end": 1700000005000 }
    },
    "delta": "增量文本"        // 流式增量 (流式模式)
  }
}
```

**Part 类型**:

| type | 说明 | 关键字段 |
|------|------|---------|
| `text` | 文本回复 | `text`, `delta`, `time` |
| `reasoning` | 推理/思考内容 | `text`, `delta`, `time` |
| `tool` | 工具调用 | `callID`, `tool`, `state` |
| `step-start` | 步骤开始 | `snapshot` |
| `step-finish` | 步骤结束 | `reason`, `cost`, `tokens` |
| `file` | 文件附件 | `url`, `mime`, `filename` |
| `snapshot` | 文件快照 | `snapshot` |
| `patch` | 文件补丁 | `hash`, `files` |
| `compaction` | 上下文压缩 | `auto` |
| `retry` | 重试 | `attempt`, `error` |
| `agent` | 子代理 | `name`, `source` |

**ToolPart 状态**:

```json
{
  "type": "tool",
  "callID": "call_01J...",
  "tool": "bash",
  "state": {
    "status": "running",  // pending | running | completed | error
    "input": { "command": "ls", "description": "List files" },
    "title": "Running bash",
    "time": { "start": 1700000000000 }
  }
}
```

#### 事件: session.status

```json
{
  "type": "session.status",
  "properties": {
    "sessionID": "session_01J...",
    "status": {
      "type": "busy",     // idle | busy | retry
      "message": "Working..."
    }
  }
}
```

#### 事件: session.idle

```json
{
  "type": "session.idle",
  "properties": {
    "sessionID": "session_01J..."
  }
}
```

#### 事件: permission.updated

```json
{
  "type": "permission.updated",
  "properties": {
    "id": "perm_01J...",
    "type": "tool_use",
    "pattern": "bash",
    "sessionID": "session_01J...",
    "title": "Allow bash command?",
    "metadata": { "command": "rm -rf /" }
  }
}
```

### 3. 提供者配置

#### GET /config/providers — 列出提供者和模型

```json
// 响应
{
  "providers": [
    {
      "id": "mimo",
      "name": "MiMo",
      "models": {
        "mimo-v2-5-pro": {
          "id": "mimo-v2-5-pro",
          "name": "MiMo v2.5 Pro",
          "reasoning": true,
          "tool_call": true,
          "limit": { "context": 200000, "output": 32000 },
          "cost": { "input": 0, "output": 0 }
        },
        "mimo-auto": {
          "id": "mimo-auto",
          "name": "MiMo Auto",
          "reasoning": true,
          "tool_call": true,
          "limit": { "context": 200000, "output": 32000 },
          "cost": { "input": 0, "output": 0 }
        }
      }
    }
  ]
}
```

#### POST /config — 更新配置

```json
{
  "activeModel": {
    "providerID": "mimo",
    "modelID": "mimo-auto"
  }
}
```

### 4. 消息获取

#### GET /session/:id/message — 获取消息列表

查询参数: `limit`, `start`, `agentID` (默认 "main")

```json
// 响应 (消息数组)
[
  {
    "id": "msg_01J...",
    "sessionID": "session_01J...",
    "role": "user",
    "agent": "main",
    "model": { "providerID": "mimo", "modelID": "mimo-auto" },
    "system": "系统提示词...",
    "tools": { "bash": true, "read": true },
    "time": { "created": 1700000000000 },
    "parts": [
      {
        "id": "part_01J...",
        "type": "text",
        "text": "用户消息"
      }
    ]
  },
  {
    "id": "msg_02J...",
    "role": "assistant",
    "parentID": "msg_01J...",
    "modelID": "mimo-auto",
    "providerID": "mimo",
    "mode": "default",
    "cost": 0.00123,
    "tokens": { "input": 500, "output": 200 },
    "finish": "stop",
    "time": { "created": 1700000001000, "completed": 1700000005000 },
    "parts": [
      { "type": "reasoning", "text": "思考过程..." },
      { "type": "text", "text": "回复内容" },
      { "type": "tool", "callID": "call_01...", "tool": "bash", "state": {...} }
    ]
  }
]
```

## 典型交互流程

```
客户端                          MiMo 服务器
  │                                │
  │  POST /session                 │
  │  ──────────────────────────>   │
  │  <──────────────────────────   │  session_id
  │                                │
  │  POST /session/:id/prompt      │
  │  {parts: [{text: "hello"}]}    │
  │  ──────────────────────────>   │
  │                                │
  │  <── SSE stream start ──────   │
  │  {"type":"session.status",     │
  │   "properties":{"type":"busy"}}│
  │                                │
  │  {"type":"message.part.updated",│
  │   "properties":{"part":{       │
  │     "type":"reasoning",        │
  │     "delta":"让我想想..."}}     │
  │  <──────────────────────────   │
  │                                │
  │  {"type":"message.part.updated",│
  │   "properties":{"part":{       │
  │     "type":"text",             │
  │     "delta":"你好！"}}          │
  │  <──────────────────────────   │
  │                                │
  │  {"type":"message.part.updated",│
  │   "properties":{"part":{       │
  │     "type":"step-finish",      │
  │     "reason":"stop",           │
  │     "cost":0.001,              │
  │     "tokens":{...}}}           │
  │  <──────────────────────────   │
  │                                │
  │  {"type":"message.updated",    │
  │   "properties":{"info":{       │
  │     "finish":"stop",...}}      │
  │  <──────────────────────────   │
  │                                │
  │  {"type":"session.idle"}       │
  │  <──────────────────────────   │
  │                                │
```

## 关键实现细节

### 1. 流式协议

- 不是标准 SSE，而是行分隔 JSON (每行一个完整 JSON 对象)
- 通过 `message.part.updated` 事件 + `delta` 字段实现增量更新
- 第一个 `message.part.updated` 包含完整 `part` 对象 + `delta`
- 后续同 part 的更新只包含 `delta` 字段

### 2. 消息格式

- 消息使用 `parts` 数组而非 `content` 字符串
- 每个 part 是 `{type, text, ...}` 结构
- 支持 image/file 附件: `{type: "file", url: "data:image/png;base64,...", mime: "image/png"}`

### 3. 工具调用

- 通过 `ToolPart` 报告工具调用: `{type: "tool", callID, tool, state: {status, input, output}}`
- 工具调用在 `text` 和 `reasoning` part 的文本中进行
- 不直接使用 OpenAI 的 `tool_calls` 格式

### 4. 模型 ID 格式

- 格式: `providerID/modelID` (如 `mimo/mimo-auto`)
- 选择模型时需分别指定 `providerID` 和 `modelID`

### 5. 认证

- 服务端: `Authorization: Basic base64("mimocode:<password>")`
- 客户端: `x-mimocode-directory` header 指定工作目录