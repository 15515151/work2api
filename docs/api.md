# 客户端接入

[返回项目首页](../README.md)

## 地址与鉴权

先在管理台或用户门户创建 API Key，从模型页复制完整模型 ID。

| 协议 | 客户端 Base URL | 请求端点 |
|---|---|---|
| OpenAI Chat | `http://127.0.0.1:8787/v1` | `POST /v1/chat/completions` |
| Anthropic Messages | `http://127.0.0.1:8787` | `POST /v1/messages` |
| OpenAI Responses | `http://127.0.0.1:8787/v1` | `POST /v1/responses` |

公网使用时将本机地址替换为你的 HTTPS 域名。Base URL 是否包含 `/v1` 取决于客户端的拼接方式，最终请求路径应与表中一致。

三种协议共用 Key 和模型路由，支持流式与非流式调用。OpenAI 兼容接口使用 `Authorization: Bearer sk-...`；Messages 也支持 `x-api-key`。`GET /v1/models` 返回当前 Key 有权使用的模型。`ADMIN_TOKEN` 是管理凭据，不用于模型调用。

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer sk-你的Key" \
  -H "Content-Type: application/json" \
  -d '{"model":"从模型页复制的完整ID","messages":[{"role":"user","content":"你好"}],"stream":true}'
```

## 模型与 Key 范围

- 渠道前缀为 `workbuddy/`、`qoder/`、`opencode/`、`trae/`；旧裸模型名仍指向 WorkBuddy。
- 同一地址和 Key 下，切换模型 ID 即可切换渠道；跨渠道共用调用名见 [统一模型路由](model-routing.md)。
- Key 的模型白名单始终生效；用户 Key 还受当前用户权限及全局禁用约束。
- 建议始终显式传入 `model`。省略时，单模型 Key 使用唯一获准模型；多模型受限 Key 返回 400；不限制模型的 Key 使用 WorkBuddy `auto`。显式 `auto` 仍需授权，它表示上游选模，不是按白名单自动选择。

Claude Code 等会使用多个模型的客户端，主模型及辅助模型均应填写网关可用的调用名。

## 协议能力

| 能力 | 说明 |
|---|---|
| 流式与非流式 | Chat、Messages、Responses 可跨协议转换 |
| 工具调用 | 保留定义、调用 ID、参数与结果，支持并行调用 |
| Responses 工具 | 支持 namespace 与 custom 的客户端类型及结果往返 |
| 技能与项目指令 | 保留客户端提交的技能、Agent、MCP 与项目规则 |
| 多模态 | 按渠道目录确认的图片与工具结果能力转发 |
| 输出状态 | 区分正常完成、输出未完成及调用失败 |

实际能力取决于客户端、模型和渠道。网关不代替客户端执行工具，也不通用模拟服务端工具、原生签名思考或 grammar 受限解码。不可转换的可选工具会被过滤，强制指定不可用工具时明确报错。

WorkBuddy 单次请求支持 `n=1`，多个 Agent 可分别发起独立请求。`POST /v1/messages/count_tokens` 提供文本和工具内容的估算，不是精确 tokenizer 或计费依据。

## 会话与输出

各渠道保留自身账号选择策略。WorkBuddy 支持成本与额度感知、国内账号偏好、会话粘性和故障切换；其他渠道差异见 [渠道接入](providers.md)。管理台「流量 → 会话」可调整后续请求的账号绑定，正在执行的请求不迁移。

优先使用客户端提供的稳定会话标识；缺失时尝试从最早用户消息识别，无法保证区分开场完全相同的会话。输出开始后不自动换号重放，客户端取消会取消对应上游读取。

OpenCode 保留客户端原生会话 ID；由网关合成的 ID 在 UTC 日期变化时刷新，跨日请求可能重新选择账号或出口。

`length` 表示输出预算或上下文边界导致未完成，不等于网络断流。状态与性能的查看方式见 [运行维护](operations.md#调用记录与续航)。
