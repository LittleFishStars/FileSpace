# 文件空间 FileSpace — Agent 指南

局域网文件共享工具：在任意文件夹执行 `filespace --web` 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

前后端合并为一个程序：**filespace**（Go 后端二进制）。不带参数只启动后端 API；带 `--web` 参数时后端托管前端静态资源（`output: 'export'` → `web/out/`，由 `go:embed` 嵌入），在浏览器中打开界面。

## 项目结构

- `web/`     — 前端（Next.js 16 App Router + antd + Tailwind v4）：`next.config.ts` 配置 `output: 'export'`（生产）与 `rewrites`（开发模式反代 /api）
- `backend/` — 后端（Go，P2P + mDNS）：`cmd/filespace`（API + `--web` 模式静态文件托管）
- `scripts/` — 构建脚本（`build.py`）与开发启动脚本（`dev.py`），Python
- `build/`   — 构建产物（gitignored）：`build/<平台>/`（后端，含嵌入的前端静态资源）

## 常用命令

```bash
make dev                    # 前端 :3000 + 后端 :8080（scripts/dev.py 自动拉起后端）
make dev-web / dev-backend  # 单独启动（dev-web 同 dev）
make build                  # 全部平台构建（等价 python3 scripts/build.py）
make build-linux / build-windows / build-darwin / build-darwin-amd64  # 指定平台交叉编译
python3 scripts/build.py windows darwin   # 脚本直接调用，可多平台
python3 scripts/build.py --list / --clean # 列出平台 / 清理产物
make pack                   # 打包安装包（等价 python3 scripts/build.py pack）
make pack-windows / pack-linux            # 指定平台安装包（msi、deb/pacman/AppImage）
```

打包依赖系统工具：wixl（msitools，Windows .msi）、dpkg-deb（dpkg）、makepkg（base-devel）、mksquashfs（squashfs-tools，AppImage 手动构建 + type2 runtime 自动缓存）、可选 rsvg-convert（librsvg）；脚本缺失时提示安装命令。安装包输出到 `build/packages/<平台>/`。

## 后端约定

- Go 1.27，模块名 `filespace`；依赖：gopsutil（跨平台采集）、zeroconf（mDNS）、yaml.v3（配置）
- 文件可预览性由**内容嗅探**判定（`http.DetectContentType`），`FileInfo.previewable` 供前端隐藏二进制文件的预览按钮，勿改回硬编码扩展名
- `--web` 模式：`go:embed` 嵌入 `backend/cmd/filespace/web/` 下的前端静态资源，`HandlerWithStatic` 组合 API 路由与静态文件服务器
- **开发模式 go run 依赖 embed 目录存在**：`//go:embed all:web` 在编译期要求 `backend/cmd/filespace/web/` 非空。仓库已提交 `.gitkeep` 占位（`git add -f` 强制跟踪，因目录被 gitignore），`scripts/dev.py` 启动后端前也会自动补齐（应对 `build.py --clean` 删除后直接 dev）；生产构建由 `build.py` 用真实静态资源覆盖该目录
- 修改依赖后 `go mod tidy`；提交前 `gofmt` + `go vet` + `go build ./...`

## 前端约定

- Next.js 16 + `output: 'export'`（生产构建产出静态文件到 `web/out/`，由后端 go:embed 嵌入托管；**动态路由段无法预渲染**，用查询参数（如 `/folders?folderId=xxx`））
- 开发模式：`next.config.ts` 的 `rewrites` 将 `/api/*` 反代到后端（http://127.0.0.1:8080）；生产模式：前端静态资源由后端直接托管，`/api/*` 同源访问无需反代
- 构建脚本 `build.py` 流程：`pnpm build`（→ `web/out/`）→ 拷贝到 `backend/cmd/filespace/web/`（go:embed 源）→ `go build` 交叉编译
- 卡片组件在 `web/app/_cards/`，页面在 `web/app/`，API 封装在 `web/app/_lib/api.ts`
- 预览渲染路由用 `@smazeeapps/file-viewer` 的 `detectFileType`（勿硬编码扩展名）；不支持的非二进制格式以纯文本显示
- **勿移除** `file_preview.tsx` 顶部的 `window.Prism = {manual: true}`（阻止 prism 自动 highlightAll 破坏代码逐行结构）
- **勿移除** `package.json` 的 `postinstall`（修补 ProLayout 内部 Drawer 废弃 width 警告）
- 主题：`AppTheme` 在 `<html>` 上开关 `.dark` class，Tailwind `dark:` 变体为 class 驱动（`@custom-variant dark`）
- 提交前 `tsc --noEmit` + `eslint` + `next build`

## 文档与提交

- 注释、文档、提交信息使用中文
- 更新 AGENTS.md / README.md 后一并提交
- 版本号约定（2026-08-30 起）：格式为 `<主>.<次>.<补丁>-<时间戳>`，当前 `backend/version.go` 为 `0.3.4-2609021622`（`web/package.json` 的 version 同步保持一致，均不带 v 前缀）。`0.3.4` 固定，**除非用户明确要求修改版本号，否则不得改动**。**提交信息中不再附带版本号**（版本号仅在用户要求升级或发版时统一修改）。
- git 推送必须用 HTTPS remote + gh 凭据助手（本环境 SSH 推送会因 ssh_config.d 权限失败）

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
