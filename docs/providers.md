# 渠道接入

[返回项目首页](../README.md)

## 支持的供应商

| provider | 上游 | 凭据来源 | 签到 | 额度 | 模型命名空间 |
|---|---|---|---|---|---|
| **workbuddy**（默认） | WorkBuddy / CodeBuddy（国内 + 国际） | 扫码 OAuth / 本机 CodeBuddy auth 文件 | ✅（国际站无活动，自动跳过） | ✅ | `workbuddy/*`，兼容裸 ID |
| **qoder** | Qoder（cn / global） | 本机 Qoder 桌面凭据自动探测（Windows）/ 扫码 OAuth / `~/.qoder2api` | ✅（每日 campaigns） | ✅（配额查询） | `qoder/*` |
| **opencode** | OpenCode Zen（zen / go 双 tier + 匿名） | WebUI「编辑配置」或 `data/opencode/opencode.json` 的 key 列表 | — | — | `opencode/*` |
| **trae** | 国内 Trae IDE / Work-SOLO | 网页 OAuth / JSON 导入 / 手动读取服务所在电脑的登录态 | ✅（按账号活动） | ✅（积分） | `trae/*` |

Trae 的 IDE 与 Work/SOLO 凭据分开保存，同一上游 UID 共用一份账号归属、额度与并发资源；仅路由到该身份实际返回的模型目录。支持用户贡献、本人调用和同渠道共享池，国际版不在当前范围。调用采用原生聊天和调用方工具路径，不启用上游远程 Agent。自动额度刷新、签到默认关闭，可在管理台启用。桌面登录态导入不修改桌面文件；需要持续刷新时建议网页登录。凭证导出不包含用户归属、分组和授权配置，完整迁移应使用完整备份。

> 未配置凭据的供应商保持 **inert**（不就绪、不列模型、不影响启动）。

> 技术栈：Go 1.26 + 标准库 `net/http` + `modernc.org/sqlite`（纯 Go，免 CGO）；Vue3 + Vite + Tailwind 自定义设计系统，`go:embed` 进二进制；无外部服务依赖，数据落在本地 SQLite。

## 各供应商接入与注意事项

### WorkBuddy / CodeBuddy（默认）
- 在管理台「账号」页筛选 WorkBuddy 后**扫码登录**（cn / 国际），或上传本机 CodeBuddy 的 `.info` auth 文件。
- **国内 CodeBuddy 桌面端新版把 token 加密存储（`$wbEncrypted`），无法离线读取**——这类账号需在 WebUI **重新扫码登录**导入明文凭据。
- 国际站 WorkBuddy 无签到活动，签到会自动跳过。

### Qoder
- **本机装了 Qoder 桌面端时会自动探测凭据**（Windows：解密 Chromium `safeStorage`，用当前用户 DPAPI），无需手动登录即可用。
- 也可在管理台「账号」页选择 Qoder 后**扫码登录**（选 cn / global），账号存到 `~/.qoder2api`。
- 若 Qoder 桌面端升级到新的 App-Bound 加密（本地无法离线解密），请改用扫码登录。
- 「设为激活」= 设置优先使用的账号。可重试故障时可切换其他满足区域和模型要求的私人账号，保留会话粘性，响应开始后不换号；不使用 WorkBuddy 的加权账号池。

### OpenCode
- **在管理台「账号 → OpenCode → 编辑配置」里填 `zen_keys` / `go_keys` / `anonymous` 等**（保存即热重载，无需重启）；也可用 `OPENCODE_CONFIG` 指定路径，或直接放 `data/opencode/opencode.json`。未配置任何 key 且未启用匿名层时，该供应商保持未就绪。
- 配置与两个模型缓存（`*.models.catalog.json` / `*.models.dev.json`）聚合在同一目录；旧版放在工作目录根的 `opencode.json` 会在启动时自动迁移到 `data/opencode/`。
- 无账号 OAuth、无签到、无额度查询（上游本身不提供），管理台仅展示 key 层与模型。
### Trae

在「账号」页添加国内 IDE 或 Work/SOLO 身份，可选择网页登录、凭证 JSON 导入或读取本机登录态。两种产品的模型目录独立获取，不把一个产品的能力推断给另一个产品。模型名统一使用 `trae/<模型>`，以管理台或 `/v1/models` 返回的真实 ID 为准。

自动签到和额度刷新默认关闭，可按需在管理台启用；未返回的价格或能力字段保持未知。国际版尚未接入。协议参考出处见 [Trae NOTICE](../internal/trae/NOTICE.md)。
