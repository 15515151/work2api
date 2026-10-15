# 下载与部署

[返回项目首页](../README.md)

## 二进制

从 [Releases](https://github.com/YuapXc/work2api/releases) 下载 Windows / Linux / macOS 对应架构的程序包，核对附带的 SHA-256，解压运行。默认监听 `127.0.0.1:8787`，管理入口为 `/admin-ui/`。

需要试用某次提交时，可从对应 [Actions](https://github.com/YuapXc/work2api/actions) 的 `Checks and commit binaries` 下载构建产物。临时包保留 14 天，版本为 `dev-<提交SHA>`，不作为正式版自动更新。

## Docker

在 `.env` 中设置自行生成的随机 `ADMIN_TOKEN`，然后运行：

```bash
docker run -d --name work2api --env-file .env \
  -p 127.0.0.1:8787:8787 -v work2api-data:/data \
  ghcr.io/yuapxc/work2api:vX.Y.Z
```

将 `vX.Y.Z` 替换为已发布版本。镜像内部监听 `0.0.0.0`，数据卷必须保留；宿主机上的反代连接 `http://127.0.0.1:8787`。纯本地 Docker HTTP 登录需显式设置 `ADMIN_COOKIE_SECURE=false`，公网 HTTPS 保持自动策略。

## HTTPS 与反向代理

- 应用设置非空随机 `ADMIN_TOKEN`，公网通过 HTTPS 访问；同机代理可用 HTTP 回源。
- 保留浏览器访问的外部 Host，包括非默认端口。Nginx 可使用 `proxy_set_header Host $http_host;`。
- 流式 API 路径关闭代理缓存与响应缓冲，设置能容纳模型等待和长流的超时。
- 管理入口按自己的部署需求保护：可使用访问控制，也可阻断公网 `/admin/`、`/admin-ui/`，通过私有入口管理。
- 真实 IP 转发只接受可信代理，代理应覆盖客户端自带的 IP 头。

`ADMIN_TOKEN` 不会自动改变监听地址；代理与应用不在同一网络命名空间时，需正确设置 `HOST` 和回源地址。

## Cloudflare Tunnel

同机部署时，已发布应用的 Service URL 使用 `http://127.0.0.1:8787`，HTTP Host Header 使用实际外部域名。域名、门户页面与 `/portal/api/` 应保持同源。

```yaml
ingress:
  - hostname: gateway.example.com
    service: http://127.0.0.1:8787
    originRequest:
      httpHostHeader: gateway.example.com
  - service: http_status:404
```

此示例转发整个站点，包括管理入口，需结合上面的管理访问控制使用。cloudflared 在其他容器中时，回源改为私有网络中的应用服务名与端口，不能使用 cloudflared 自己的 `127.0.0.1`。

登录出现跨源拒绝时先检查回源 Host，不要关闭来源校验或放开通配 CORS。502 等问题见 [运行维护](operations.md#常见问题)。

## 数据持久化

数据、凭据、主密钥和备份均应保密，不作为静态目录发布或提交 Git。更新前生成 [完整备份](operations.md)，保留原数据和配置。正式版安装能力见 [版本更新](operations.md#版本更新)。
