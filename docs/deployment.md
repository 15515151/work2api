# 下载与部署

[返回项目首页](../README.md)

## 下载与部署（免手动编译）

推荐直接用**预编译产物**，不必本地 `go build`：

### 方式 A：下载 Release 二进制

打 tag（`vX.Y.Z`）后，GitHub Actions 会交叉编译 Windows / Linux / macOS（amd64 + arm64）并发布到 [Releases](https://github.com/YuapXc/work2api/releases)，每个包附 `.sha256` 校验和。下载对应平台的压缩包，校验后解压即用：

```powershell
# Windows：校验后运行
Get-FileHash .\work2api_vX.Y.Z_windows_amd64\work2api.exe -Algorithm SHA256
.\work2api.exe
```

### 方式 B：Docker（配合 HTTPS 反代或 Tunnel）

```bash
docker run -d --name work2api --env-file .env -p 127.0.0.1:8787:8787 \
  -v work2api-data:/data \
  ghcr.io/yuapxc/work2api:vX.Y.Z
# .env 只需设置 ADMIN_TOKEN；容器内保留 HOST=0.0.0.0。
# 宿主机的反代/Tunnel 连接 127.0.0.1:8787，浏览器通过 HTTPS 域名访问。
```

> 本地使用无需配置。公网 HTTPS 部署只需设置 `-e ADMIN_TOKEN=<你的随机令牌>`：应用自动允许外部 Host，并为公网域名启用 Secure Cookie。监听地址不会因此改变；容器镜像已监听 `0.0.0.0`，主机反代或 Tunnel 可以连接默认回环地址。

公网域名应通过 HTTPS 访问，支持代理以 HTTP 回源；反向代理须保留外部 `Host`（含非默认端口），以便校验浏览器请求的 `Origin`。自动模式下，本机与 SSH 隧道通过 localhost/回环 IP 访问可继续使用 HTTP，前提是实际连接也来自回环地址。显式 `ALLOW_EXTERNAL_HOST` 和 `ADMIN_COOKIE_SECURE` 配置优先于自动默认值；例如仅可信局域网直接使用 HTTP 时才设置 `ADMIN_COOKIE_SECURE=false`。服务不会凭 `X-Forwarded-Proto` 放宽 Cookie 或来源校验。脚本管理请求继续使用 `X-Admin-Token`，分发给调用者的应是独立 API Key。

#### Cloudflare Tunnel 部署门户

公网门户与管理 API 使用同源校验。应用进程只需设置非空随机 `ADMIN_TOKEN`，外部 Host 与公网 Secure Cookie 默认自动开启；应配置在 work2api 进程或容器上，而不是 cloudflared 上。若旧配置显式设置了 `ALLOW_EXTERNAL_HOST=false` 或 `ADMIN_COOKIE_SECURE=false`，需移除这些覆盖项。保持门户页面与 `/portal/api/` 使用同一外部域名。

如果 cloudflared 与 work2api 运行在同一台主机、同一网络命名空间，可在隧道的已发布应用路由中设置：

- Service URL：`http://127.0.0.1:8787`，不要给没有 TLS 的应用端口填 `https://`。
- HTTP Host Header：实际门户域名，例如 `gateway.example.com`，不带协议或路径；使用非默认端口时包含端口。

本地管理的隧道可将以下片段合并到已有配置（域名需替换）：

```yaml
ingress:
  - hostname: gateway.example.com
    path: ^/(admin|admin-ui)(/|$)
    service: http_status:404
  - hostname: gateway.example.com
    service: http://127.0.0.1:8787
    originRequest:
      httpHostHeader: gateway.example.com
  - service: http_status:404
```

上面的路由先阻断公开管理入口，管理台仍通过本机或私有隧道访问。使用控制台管理的隧道时，同样将管理路径阻断规则放在一般转发规则之前；也可由已有 Nginx 阻断 `/admin/` 与 `/admin-ui/`。

如果 cloudflared 在另一容器中，`127.0.0.1` 指向该容器自身；改用同一私有 Docker 网络内的应用服务名及端口，并确保应用在容器内监听 `0.0.0.0`。中间还有 Nginx 时，继续保留外部 Host（例如 `proxy_set_header Host $http_host;`）。不要通过禁用来源校验、关闭 Secure Cookie 或放开通配 CORS 解决公网登录问题。

遇到“拒绝跨源请求”，先核对 HTTPS 页面域名与回源 Host 是否一致，以及旧配置是否覆盖了自动安全策略。遇到 HTML 502，检查 cloudflared 的回源地址、端口、协议以及反代日志；遇到应用 JSON 502 且提示“发起登录失败”，检查应用到授权上游的连通性。这些错误需要分别定位。

参考：[Cloudflare 回源参数](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/configure-tunnels/origin-parameters/)、[Tunnel 排错](https://developers.cloudflare.com/tunnel/troubleshooting/)。

## 安全边界

- 对外暴露：默认仅监听回环。公网 HTTPS 部署只需设置随机 `ADMIN_TOKEN`，自动允许外部 Host 并启用公网 Secure Cookie；直接接收外部连接时另设 `HOST=0.0.0.0`。反代保留外部 `Host`，切勿无鉴权暴露。
- 真实客户端 IP：应用仅接受明确可信即时代理的 `X-Real-IP`；反代须强制覆盖此头（不要透传客户端自带值），边缘加速（如 EdgeOne）应用官方回源范围或私密回源鉴权验证。
- 门户登录/注册限流：每 IP 60 次、整体 300 次/10 分钟，登录另有每用户名 10 次/10 分钟。
- 敏感数据：`data/`（DB、含明文对话的使用记录）、`auths/`、`*.db`、`.secret_key`、`*.info` 均含凭据/隐私，已在 `.gitignore` 中，切勿提交或外传；备份主密钥须与数据库同源。
宿主机端口映射通常会让容器看到非回环的实际连接地址，因此不能把它当作应用内的真实回环 HTTP 例外。纯本地 Docker HTTP 部署需显式设置 `ADMIN_COOKIE_SECURE=false`；公网 HTTPS 保持自动策略。管理员仍需使用受保护的管理入口。

其他配置见 [配置与调用容量](configuration.md)，更新与数据迁移见 [运行维护与备份](operations.md)，异常排查见 [故障排查](troubleshooting.md)。
