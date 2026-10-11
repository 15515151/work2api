# 配置与调用容量

[返回项目首页](../README.md)

本地通常无需配置。公网 HTTPS 部署通常只需设置随机 `ADMIN_TOKEN`；监听地址、端口、数据目录有需要时再修改。配置模板见 [`.env.example`](../.env.example)，运行参数优先级为命令行 > 环境变量 > `.env`。

旧配置中的显式值会覆盖新的自动默认值；想使用自动策略时，删除对应覆盖项后重启。反代与 Tunnel 接线见 [部署说明](deployment.md)。


## 配置（flag > 环境变量 > .env）

| flag | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `-host` | `HOST` | `127.0.0.1` | 监听地址 |
| `-port` | `PORT` | `8787` | 监听端口 |
| `-data-dir` | `DATA_DIR` | `data` | 数据目录（DB / 凭据 / 密钥） |
| `-db` | `DB_PATH` | `<data-dir>/work2api.db` | SQLite 路径 |
| `-admin-token` | `ADMIN_TOKEN` | 空 | 管理台/`/admin/*` 令牌；**空 = 仅回环可用** |
| `-log-level` | `LOG_LEVEL` | `INFO` | 日志级别 |
| — | `ALLOW_EXTERNAL_HOST` | 自动 | 有 ADMIN_TOKEN 默认开启，否则关闭；显式值可覆盖（防 DNS rebinding） |
| — | `ADMIN_COOKIE_SECURE` | 自动 | 有令牌/允许外部 Host 时默认开启；真实回环 HTTP 自动兼容，显式值覆盖；直连 HTTPS 始终 Secure |
| — | `MAX_REQUEST_BYTES` | `67108864` | 请求体上限（64 MiB，必须大于 0）；登录固定 8 KiB，读取期限 30 秒 |
| — | `MODEL_BUFFER_BUDGET_BYTES` | 自动 | 响应上限 × 4 × 全局执行并发；显式值可收紧 |
| — | `MODEL_MEMORY_GUARD` / `MODEL_MEMORY_HIGH_BYTES` | `false` / `268435456` | 默认关闭进程内存压力闸，主动启用时使用配置阈值 |
| — | `MAX_CONCURRENT_REQUESTS` | `13` | 模型执行并发（含 SSE 与管理台模型测试）；管理接口独立限额 |
| — | `MODEL_QUEUE_SIZE` / `MODEL_QUEUE_WAIT_SECONDS` | `32` / `60` | 有界排队与累计等待预算，包含账号节流与重新入池 |
| — | `MODEL_KEY_QUEUE_SIZE` / `MODEL_KEY_CONCURRENCY_LIMITS` | `16` / 空 | 每 Key 等待上限；JSON 按应用 ID 配置执行上限，例如 `{"2":1}` |
| — | `ADMIN_CONCURRENCY` / `HEAVY_ADMIN_CONCURRENCY` | `8` / `2` | 轻管理 / 重管理独立限额；同类刷新共享执行结果 |
| — | `QUERY_CONCURRENCY` / `BODY_READ_CONCURRENCY` | `4` / `4` | 模型列表及 Token 统计 / 请求体读取限额 |
| — | `REQUEST_BODY_BUDGET` | 自动 | 取 256 MiB 与单请求上限 × 3 中较大者；执行与排队共享原始缓冲预算 |
| — | `MAX_JSON_ITEMS` / `MAX_JSON_DEPTH` | `100000` / `128` | 解码前限制 JSON 结构符号数量与嵌套深度，防止对象数量放大内存 |
| — | `MAX_RESPONSE_BYTES` | `8388608` | 每次上游响应上限，SSE 含封装，防止输出聚合无界增长 |
| — | `DESENSITIZE` | 已停用 | 旧配置忽略；完整保留技能、工具和项目指令，不再摘要替换或插入零宽字符 |
| — | `CHANNEL_IDENTITY_COMPAT` | `true` | 国内明确渠道拒绝且未开始输出时，兼容已验证的 Claude Code CLI/SDK 固定身份声明；保留技能、工具、权限和任务内容；关闭后同时关闭归因精简 |
| — | `CHANNEL_METADATA_COMPAT` | `true` | 身份兼容仍被拒绝时，再移除已验证的 CC billing 归因行；两级兼容合计最多 2 次，连同授权范围内换号共最多 5 次。成功方式仅在当前会话、账号及系统/工具模板一致时复用；重启失效 |
| — | `RATELIMIT` / `RATELIMIT_INTERVAL` | `true` / `1.5` | 每账号限速 |
| — | `CHECKIN_HOURS` | `9,21` | 每日自动签到小时 |
| — | `CREDIT_REFRESH_MIN` | `30` | 额度刷新周期（分钟） |
| — | `MODEL_REFRESH_HOUR` | `6` | 每日模型刷新小时 |
| — | `KEEPALIVE_HOUR` | `22` | 每日 token 保活小时 |
| — | `USAGE_RETENTION_DAYS` | `90` | 使用记录保留天数 |
| — | `USAGE_MAX_ROWS` | `100000` | 使用记录总行数上限（0=不限），兜住高频调用把库撑爆 |
| — | `USAGE_CHECK_INTERVAL_MIN` | `30` | 行数上限检查周期（分钟，MIN/MAX id 廉价预检） |
| — | `USAGE_CONTENT_MAX_BYTES` | `32768` | 单条对话内容截断上限（0=不限），主要截断 base64 图片等大 payload |
| — | `LOG_TO_FILE` | `1` | 日志是否写文件（`0` 关闭） |
| — | `OPENCODE_CONFIG` | `<data-dir>/opencode/opencode.json` | OpenCode 配置文件路径（显式设置优先） |
| — | `QODER2API_HOME` | `~/.qoder2api` | Qoder 数据目录 |
| — | `PORTAL_ENABLED` | `true` | 用户门户总开关；关闭后注册/贡献/共享调用停用，登录与查看保留 |
| — | `PORTAL_REGISTRATION_MODE` | `invite` | 注册方式（兼容旧名 `PORTAL_INVITE_MODE`） |
| — | `PORTAL_USER_CONCURRENCY` / `PORTAL_SHARED_CONCURRENCY` | `4` / `12` | 门户每用户 / 共享池执行上限 |
| — | `PORTAL_USER_QUEUE_SIZE` / `PORTAL_SHARED_QUEUE_SIZE` | `6` / `28` | 门户每用户 / 共享池等待上限 |
| — | `WORKBUDDY_ACCOUNT_CONCURRENCY` | `4` | 每个 WorkBuddy 账号 UID 执行上限（跨模型、私人/共享合并） |
| — | `PORTAL_KEY_MAX_PER_USER` / `PORTAL_MAX_TASKS_PER_USER` | `2` / `3` | 每用户 Key 数 / 同时扫码任务数 |
| — | `PORTAL_DAILY_REQUESTS` / `PORTAL_DAILY_INPUT_TOKENS` / `PORTAL_DAILY_OUTPUT_TOKENS` | `0` | 门户每日请求 / 输入 / 输出预算（0=不限） |

## 并发与资源准入

默认全局执行硬上限 13、共享池 12、每用户 4、每个 WorkBuddy 账号 UID 4（跨模型及私人/共享池合并）。响应预留预算默认自动覆盖全部配置的执行名额：按响应上限 × 4 × 全局并发计算；实际流式按响应上限 2 倍预留、非流式 4 倍，保留一个私人执行槽及其响应预算。Go 内存压力闸默认关闭，不因大请求解码后的暂时占用暂停调用；受限内存环境可显式启用 `MODEL_MEMORY_GUARD=true` 并设置合适的 `MODEL_MEMORY_HIGH_BYTES`。等待上限：全局 32 / 共享 28 / 每用户 6，累计等待预算 60 秒（含账号节流等待），反向代理超时须覆盖排队及模型首包。

Agent 长会话的请求体默认可达 64 MiB，原始缓冲预算自动联动；进一步调整时通常只需设置 `MAX_REQUEST_BYTES`。`BODY_READ_CONCURRENCY` 控制同时读取的请求数，不控制单个请求大小，无需为放行大请求而调高。旧 `.env` 显式填写的预算或 `MODEL_MEMORY_GUARD=true` 仍优先，迁移到自动默认值时需删除这些覆盖项。模型自身的上下文上限仍由上游决定。

本地过载返回 HTTP 429、`error.type=local_overload`、具体 `error.code`（如 `model_queue_full`、`key_queue_full`、`model_queue_timeout`）与 `Retry-After: 2`；调用端仅对此类执行前拒绝加随机退避重试，最多 2 次。多 Key 按用户合并计数，取消/超时/断连及时回收名额；账号忙时排队，不因忙碌换号。每日次数与 token 预算默认不限（0）。

多 Agent 场景：个人 Key 可使用全部空闲名额，分发 Key 可用 `MODEL_KEY_CONCURRENCY_LIMITS` 按应用 ID 设较低上限。原始请求体预算不代表进程内存上限（JSON 解码、协议转换、图片、响应序列化仍会放大）；提高硬上限或请求/响应上限前，应在目标服务器以长上下文、图片和非流式响应实测峰值 RSS 与延迟。
