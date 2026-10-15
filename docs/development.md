# 开发、验证与发布

[返回项目首页](../README.md)

## 源码运行

需要 Go ≥ 1.26。前端已内嵌，纯后端构建不需要 Node。

```bash
go build -o work2api ./cmd/server
./work2api
```

Windows 使用 `go build -o work2api.exe ./cmd/server`，也可双击 `scripts/run.cmd`，或运行：

```powershell
pwsh -File scripts/run.ps1
pwsh -File scripts/run.ps1 -NoRun
```

## 修改前端

需要 Node ≥ 22，在项目根目录执行：

```bash
npm --prefix webui ci
npm --prefix webui run build:embed
npm --prefix webui run check:embed
```

`build:embed` 包含类型检查、构建、同步内嵌资源与清理旧哈希文件。修改前端后应同时提交源码和 `internal/app/webui`，再重建 Go 程序。源码统一使用 LF，避免跨平台构建哈希不同。样式规范见 [WebUI 设计约定](../webui/DESIGN.md)。

## 验证与产物

```bash
go test ./...
go vet ./...
```

每次分支推送通过检查后，Actions 生成六平台临时包，保留 14 天。

正式版仅手动运行 `Release binaries and GHCR image` 并填写 `vX.Y.Z` 发布，普通 push 或推送 tag 不发布 Release。标题固定为版本号，发行说明自动列出上一版本以来的非合并提交标题，并附对比链接。
