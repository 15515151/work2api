# 开发与构建

[返回项目首页](../README.md)

## 从源码构建

### 1. 环境要求

- **Go ≥ 1.26**（Windows 默认安装在 `C:\Program Files\Go\bin`，若未加入 PATH 需自行指定）
- 仅当**要改前端**时才需要 **Node ≥ 22**（前端产物已 `embed` 进仓库，纯后端构建无需 Node）

### 2. 编译后端（前端已内置）

```bash
# 前端 dist 已 embed 进 internal/app/webui/，直接编译即可
CGO_ENABLED=0 go build -o work2api.exe ./cmd/server
./work2api.exe                     # 默认监听 127.0.0.1:8787
./work2api.exe -port 8799          # 自定义端口
```

Windows 上也可用脚本一键构建并运行：

双击 `scripts/run.cmd` 即可启动并打开管理面板；兼容 Windows PowerShell 5.1，报错时窗口会保留错误信息。终端中也可使用以下命令：

```powershell
pwsh -File scripts/run.ps1          # 构建 work2api.exe 并运行
pwsh -File scripts/run.ps1 -NoRun   # 只构建
```

### 3.（可选）重建前端

只有修改 `webui/` 源码后才需要：

```bash
cd webui
npm ci
npm run build:embed                 # 类型检查、构建并同步内嵌产物
npm run check:embed                 # 校验内嵌资源与 dist 一致
# 回到项目根目录，重新构建后端
```
## 验证

在项目根目录运行 `go test ./...` 与 `go vet ./...`。修改前端后先运行 `npm --prefix webui run build:embed`，再运行 `npm --prefix webui run check:embed`。同步工具会清理已失去引用的旧构建产物；不要手工保留多批哈希资源。
