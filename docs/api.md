# API 与客户端接入

[返回项目首页](../README.md)

## API 端点

- 推理（需 `Authorization: Bearer sk-...`）：`POST /v1/chat/completions`、`POST /v1/messages`、`POST /v1/responses`、`GET /v1/models`
- 管理（按初始化状态验证管理员会话或 `X-Admin-Token`；只有未初始化、未配置 Token 的真实回环模式允许对应本机入口）：`GET /admin/providers`、`GET/POST /admin/providers/{name}/...`（账号/模型/签到/额度/OAuth/账号管理）、`/admin/apps`、`/admin/apps/{id}/models`（密钥模型白名单）、`/admin/models/catalog`（跨供应商聚合模型目录）、`/admin/checkin/history`（跨供应商签到日历）、`/admin/usage/*`、`/admin/settings`、`/admin/login` + `/admin/logout`（WebUI 会话）
- 健康检查：`GET /health`（只报计数，不含账号 UID）；`GET /admin/health`（管理台健康详情）

## 接入客户端（三协议 base URL + 供应商前缀路由）

work2api 同时开三种协议，客户端按实际支持的协议接入；具体能力边界见 [功能与协议能力](features.md)。base URL 按客户端习惯填：

| 协议 | base URL | 客户端自动拼的路径 | 谁用 |
|---|---|---|---|
| Anthropic Messages | `http://127.0.0.1:8787` | `/v1/messages` | Claude Code、Claude 系客户端 |
| OpenAI Chat | `http://127.0.0.1:8787/v1` | `/v1/chat/completions` | 多数 OpenAI 兼容工具（Cherry Studio、LobeChat 等） |
| OpenAI Responses | `http://127.0.0.1:8787/v1` | `/v1/responses` | Codex CLI 等用 Responses API 的客户端 |

三个端点**共用同一套账号池、同一个 `sk-...`、同一套前缀路由**，协议之间由 work2api 自动互转。

**多供应商靠模型名前缀分流**——切供应商只换模型名，不换地址/Key：

| 供应商 | 模型名写法 |
|---|---|
| WorkBuddy（默认） | `workbuddy/<模型>`，兼容 `<模型>`（无前缀） |
| Qoder | `qoder/<模型>` |
| OpenCode | `opencode/<模型>` |
| Trae | `trae/<模型>` |

> 以 Claude Code + [cc-switch](https://github.com/farion1231/cc-switch) 这类切换器为例：按需建立 profile，**base URL 和 Key 完全相同**，只有模型名前缀不同即可切换渠道。注意 Claude Code 会同时用主模型（`ANTHROPIC_MODEL`）和小快模型（`ANTHROPIC_SMALL_FAST_MODEL`）——两个都要填成 work2api 能路由的模型（同一家、同前缀），否则后台调用会因模型不存在报错。

## 模型授权与默认模型

- **密钥模型白名单**：`/v1/models` 只展示该密钥获准调用的模型及别名。省略 `model`（含 `null`、空字符串）时，不限模型的密钥默认使用 `auto`，仅允许一个模型的密钥默认使用该模型，允许多个模型的密钥需明确指定，否则返回 400。显式指定 `auto` 仍需获得授权；它会交给上游选择，并不表示在密钥白名单内自动选择。

`/v1/messages/count_tokens` 当前提供文本及工具内容的粗估（含 system、工具定义和结果），不使用供应商 tokenizer；图片和音频未计入，不能作为精确计费或上下文容量依据。
