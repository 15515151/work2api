# work2api

将 WorkBuddy / CodeBuddy、Qoder、OpenCode 和国内 Trae 整合到一个 Go 网关，对外提供 OpenAI Chat、Anthropic Messages、OpenAI Responses 三种兼容 API，内置管理台和用户门户。

各渠道独立配置；没有 WorkBuddy 账号也能使用其他已配置渠道。前端内嵌在二进制中，数据保存在本地 SQLite，无需外部数据库。

> 仅供学习与技术交流。使用前请阅读 [使用声明](docs/usage-policy.md)；账号和凭据由使用者自行提供。

## 主要能力

- 三协议的流式与非流式调用、工具调用和协议转换。
- 独立 API Key、模型白名单、账号路由、会话粘性与故障换号。
- 按上游实际信息展示模型能力、额度、资源包与用量；缺失字段保留未知。
- 管理台统一管理各渠道；用户门户支持注册、贡献账号和自助创建 Key。
- 在线完整备份、定时维护与调用诊断。

兼容范围和各渠道差异见 [功能说明](docs/features.md) 与 [渠道接入](docs/providers.md)。

| 渠道 | 模型名 | 用户贡献 / 共享 |
|---|---|---|
| WorkBuddy / CodeBuddy | `workbuddy/<模型>`，兼容裸模型名 | 支持 |
| Qoder | `qoder/<模型>` | 暂不开放 |
| OpenCode | `opencode/<模型>` | 暂不开放 |
| 国内 Trae IDE / Work-SOLO | `trae/<模型>` | 支持 |

## 快速开始

1. 从 [Releases](https://github.com/YuapXc/work2api/releases) 下载对应平台的二进制，校验 SHA-256 后解压运行；默认地址为 `127.0.0.1:8787`。
2. 打开 `http://127.0.0.1:8787/admin-ui/`，按页面提示初始化管理员或登录。
3. 在「账号」页配置渠道，在「API 密钥」页创建独立 Key，从「模型」页复制可用模型 ID。
4. 客户端使用该 Key 和模型 ID 接入；不同协议的地址写法见 [客户端接入](docs/api.md)。

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer sk-你的key" \
  -H "Content-Type: application/json" \
  -d '{"model":"替换为模型页复制的完整ID","messages":[{"role":"user","content":"你好"}]}'
```

本地通常无需 `.env`。公网 HTTPS 部署通常只需设置随机 `ADMIN_TOKEN`；反代或 Tunnel 保留外部 Host，阻断公开管理入口。Docker、Tunnel 和配置覆盖说明见 [部署文档](docs/deployment.md)。

源码启动需要 Go ≥ 1.26；Windows 可双击 `scripts/run.cmd`，其他平台运行 `go build -o work2api ./cmd/server` 后启动。修改前端时才需要 Node ≥ 22，详见 [开发与构建](docs/development.md)。

## 文档

| 文档 | 内容 |
|---|---|
| [渠道接入](docs/providers.md) | 凭据来源、各渠道配置和调用差异 |
| [API 与客户端](docs/api.md) | 三协议地址、模型前缀、Key 授权与默认模型 |
| [用户门户与管理员](docs/portal.md) | 注册、共享资格、管理员权限与恢复入口 |
| [下载与部署](docs/deployment.md) | Release、Docker、HTTPS、Cloudflare Tunnel 与安全边界 |
| [配置与调用容量](docs/configuration.md) | 环境变量、自动默认值、并发与请求预算 |
| [功能与协议能力](docs/features.md) | 工具、技能、多模态、缓存与响应完整性 |
| [运行维护与备份](docs/operations.md) | 实例复用、更新、诊断、在线备份与迁移 |
| [开发与构建](docs/development.md) | 后端编译、前端同步与验证 |
| [故障排查](docs/troubleshooting.md) | 413、502、跨源拒绝、11128、端口和构建问题 |
| [使用声明](docs/usage-policy.md) | 原有免责声明与使用边界 |

## 参考项目

- [Joy4Fire/Workbuddy2API](https://github.com/Joy4Fire/Workbuddy2API)
- [Zhengyuuuui/qoder2api](https://github.com/Zhengyuuuui/qoder2api)
- [jasonxu114514/opencode2api](https://github.com/jasonxu114514/opencode2api)
- Trae 协议移植参考 [mmqz/cpa-multi-plugins](https://github.com/mmqz/cpa-multi-plugins)（Copyright 2026 mmqz，MIT）和 [jsongo/buddy-proxy](https://github.com/jsongo/buddy-proxy)（Copyright 2026 Mayer，MIT）；前者标注国内 SOLO 协议源自 Sliverkiss/traework2api，并对照 connectedGraph/trae2api-web。完整来源与许可见仓库 [NOTICE](https://github.com/YuapXc/work2api/blob/main/internal/trae/NOTICE.md) 和 [LICENSE](https://github.com/YuapXc/work2api/blob/main/internal/trae/LICENSE)。
