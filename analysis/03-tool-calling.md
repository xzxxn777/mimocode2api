# MiMo-Code Tool Calling 机制分析

## 概述

MiMo-Code 的工具调用通过 Vercel AI SDK 的 `tool()` 函数注册，在 OpenCode 协议中通过 `ToolPart` 事件报告。

## 工具注册

```typescript
// tool/registry.ts
const BUILTIN_TOOLS = {
  bash:        // 执行 shell 命令
  bash_interactive: // 交互式 shell
  read:        // 读取文件
  write:       // 写入文件
  edit:        // 编辑文件 (diff-based)
  multiedit:   // 批量编辑
  glob:        // 文件匹配
  grep:        // 内容搜索
  task:        // 子任务
  todo:        // TODO 管理
  webfetch:    // 获取网页
  websearch:   // 网络搜索
  question:    // 向用户提问
  plan:        // 计划
  skill:       // 技能调用
  lsp:         // LSP 操作
  codesearch:  // 代码搜索
  memory:      // 记忆管理
  actor:       // 子代理管理
}
```

## 工具调用协议

### 在 OpenCode 协议中的表示

工具调用通过 `message.part.updated` 事件中的 `ToolPart` 表示：

```json
{
  "type": "message.part.updated",
  "properties": {
    "part": {
      "id": "part_01J...",
      "sessionID": "session_01J...",
      "messageID": "msg_01J...",
      "type": "tool",
      "callID": "call_01J...",
      "tool": "bash",
      "state": {
        "status": "completed",
        "input": {
          "command": "ls -la",
          "description": "List files in directory"
        },
        "output": "total 48\ndrwxr-xr-x  12 user  staff  384...",
        "title": "Ran bash command",
        "time": {
          "start": 1700000000000,
          "end": 1700000001000
        }
      }
    }
  }
}
```

### 工具状态转换

```
pending → running → completed
                 → error
```

### AI SDK 层的工具调用

在 Vercel AI SDK 层面，工具调用使用标准的 OpenAI `tool_calls` 格式：

```typescript
// AI SDK 自动处理 tool_call → tool 执行 → tool_result 的循环
const result = await generateText({
  model,
  messages,
  tools: {
    bash: tool({
      description: "Execute a shell command",
      parameters: z.object({
        command: z.string().describe("The command to execute"),
        description: z.string().describe("Brief description")
      }),
      execute: async ({ command }) => {
        // 实际执行命令
        return execSync(command).toString()
      }
    })
  }
})
```

## MiMo 模型的工具调用特性

### 关键发现

1. **MiMo 模型不输出标准 OpenAI tool_calls 格式**
   - MiMo 模型 (mimo-auto, mimo-v2-5-pro 等) 在响应中不生成标准的 `tool_calls` 数组
   - 而是通过文本描述来表达工具调用意图
   - 这导致 AI SDK 的自动 tool calling 循环无法正常工作

2. **解决方案: Hermes system prompt 管理**
   - 将工具调用管理交给 Hermes Agent 的 system prompt
   - 代理层不注入工具 prompt 或桥接外部工具
   - 配置:
     ```
     MIMOCODE_DISABLE_TOOLS=true
     MIMOCODE_PROXY_OMIT_SYSTEM_PROMPT=false
     MIMOCODE_PROXY_PROMPT_MODE=standard
     ```

3. **工具调用文本格式**
   MiMo 模型可能输出类似以下格式的工具调用:
   ```
   <function_calls>
   <invoke name="bash">
   <parameter name="command">ls -la</parameter>
   </invoke>
   </function_calls>
   ```
   但这不是标准格式，且模型不总是遵循此格式。

### 对代理实现的影响

由于 MiMo 模型不输出标准 tool_calls，代理有两种处理策略:

**策略 A: 禁用工具 (推荐)**
- 代理完全不转发工具定义
- Hermes 通过 system prompt 管理工具调用
- 代理只做纯文本翻译

**策略 B: Prompt injection**
- 代理在 system prompt 中注入工具描述
- 解析模型输出中的 XML/JSON 格式工具调用
- 可靠性低 (免费模型几乎不遵循)

## 代理实现中的工具处理

### 请求转换

```go
// OpenAI ChatCompletionRequest → OpenCode parts
// 忽略 tools 和 tool_choice 字段
// tool role 消息转为 user 消息:
//   role: "tool" → role: "user", content: "[Tool Result: ...]"
```

### 响应转换

```go
// OpenCode SSE events → OpenAI SSE chunks
// stream 模式:
//   reasoning → choices[0].delta.content = " thinking\n{text}"
//   text → choices[0].delta.content = "{text}"
//   tool → 忽略 (MiMo 不输出标准 tool_calls)
//   step-finish → finish_reason = "stop"
//   message.updated → 发送最终 chunk + [DONE]
//
// non-stream 模式:
//   收集所有 content → 组装完整 JSON 响应
```

## 与 Hermes Agent 的协作

```
Hermes Agent
    │
    │  system prompt (含工具定义)
    │  POST /v1/chat/completions
    │  {model: "mimo/mimo-auto", messages: [...], tools: [...], stream: true}
    ▼
MiMo Proxy (Go)
    │
    │  tools 被忽略 (不转发给 MiMo)
    │  messages 转换: OpenAI → OpenCode parts
    │  POST /session/:id/prompt
    ▼
MiMo 服务器
    │
    │  模型回复纯文本
    │  SSE events (text, reasoning, finish)
    ▼
MiMo Proxy (Go)
    │
    │  OpenCode SSE → OpenAI SSE 转换
    │  data: {"choices":[{"delta":{"content":"..."}}]}
    ▼
Hermes Agent
    │
    │  解析文本回复
    │  通过 system prompt 判断是否需要工具调用
    │  如果需要 → 执行工具 → 将结果追加到 messages
    │  继续下一轮对话
```

## 总结

- MiMo 模型不原生支持工具调用 (不输出标准 tool_calls)
- 工具调用管理完全交给 Hermes Agent 的 system prompt
- 代理层只做消息格式转换，不参与工具调用逻辑
- 这种设计简单可靠，避免了 prompt injection 的不可靠性