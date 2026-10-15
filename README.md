# work2api

将 **WorkBuddy / CodeBuddy、Qoder、OpenCode、国内 Trae IDE / Work-SOLO** 接入同一个 2api 网关，提供 OpenAI Chat、Anthropic Messages 和 OpenAI Responses 兼容接口。

Go 单程序内置管理台与用户门户，使用本地 SQLite，无需外部数据库。各渠道独立配置，不依赖 WorkBuddy 账号启动。

## 项目特色

- **一套地址与 Key，调用多个渠道**：通过模型前缀切换渠道，也可手工合并为统一模型名。
- **保留客户端工具能力**：支持流式输出、工具调用、多模态和跨协议转换，适配 function、namespace 与 custom 工具。
- **多账号调度**：保留各渠道的成本、区域、额度、会话粘性和故障切换策略；可在管理台调整会话后续命中账号。
- **用户自助使用**：注册后自行创建 Key，可选贡献个人账号；管理员配置默认模型范围与独立授权。
- **动态模型与用量展示**：展示上游模型能力、优惠规则、账号资源包、调用性能和多渠道续航估算。
- **内置维护**：额度刷新、签到、日志清理、在线完整备份及版本自更新。

具体协议能力见 [客户端接入](docs/api.md)，各渠道差异见 [渠道接入](docs/providers.md)。

## 快速开始

1. 从 [Releases](https://github.com/YuapXc/work2api/releases) 下载对应系统与架构的程序，校验后解压运行。
2. 打开 `http://127.0.0.1:8787/admin-ui/`，按提示初始化管理员。
3. 在「账号」页添加渠道账号，在「API 密钥」页创建 Key。
4. 从「模型」页复制调用名，按 [客户端接入](docs/api.md) 配置客户端。

本地通常无需 `.env`。公网部署、Docker 与 Cloudflare Tunnel 配置见 [部署说明](docs/deployment.md)。源码构建和 Windows 一键启动见 [开发与构建](docs/development.md)。

## 使用文档

| 主题 | 文档 |
|---|---|
| 模型调用 | [客户端接入](docs/api.md) · [渠道接入](docs/providers.md) |
| 账号与权限 | [用户门户与管理员](docs/portal.md) · [统一模型路由](docs/model-routing.md) |
| 部署与配置 | [下载与部署](docs/deployment.md) · [配置说明](docs/configuration.md) |
| 日常维护 | [更新、备份与排障](docs/operations.md) |
| 开发 | [开发、验证与发布](docs/development.md) |

**仅供学习与技术交流。** 使用者须遵守各渠道服务条款及适用法律法规，自行承担部署和使用责任；项目作者、维护者及贡献者在适用法律允许的范围内不承担相关损失责任。使用前请阅读 [使用声明与免责声明](docs/usage-policy.md)。

## 参考与来源

- [Joy4Fire/Workbuddy2API](https://github.com/Joy4Fire/Workbuddy2API)
- [Zhengyuuuui/qoder2api](https://github.com/Zhengyuuuui/qoder2api)
- [jasonxu114514/opencode2api](https://github.com/jasonxu114514/opencode2api)
- Trae 协议参考 [mmqz/cpa-multi-plugins](https://github.com/mmqz/cpa-multi-plugins)（Copyright 2026 mmqz，MIT）与 [jsongo/buddy-proxy](https://github.com/jsongo/buddy-proxy)（Copyright 2026 Mayer，MIT）。完整来源及许可见 [NOTICE](internal/trae/NOTICE.md) 与 [LICENSE](internal/trae/LICENSE)。
