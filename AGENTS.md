# 文件空间 FileSpace — Agent 指南

局域网文件共享工具：在任意文件夹执行前端程序即可共享该文件夹（自动拉起后端），打开浏览器即可查看局域网内所有已共享的文件夹。

前后端是两个独立程序：**filespace**（后端，纯 API）+ **前端**（Next.js standalone 服务器，即前端本身，运行入口 `node web/start.js`，安装包命令 `filespace-web`）。

## 项目结构

- `web/`     — 前端（Next.js 16 App Router + antd + Tailwind v4）：`proxy.ts` 反代 /api、`start.js` 前端引导
- `backend/` — 后端（Go，P2P + mDNS）：`cmd/filespace`（API）
- `scripts/` — 构建脚本（`build.py`）与开发启动脚本（`dev.py`），Python
- `build/`   — 构建产物（gitignored），`build/<平台>/` 下为 filespace + web/（standalone 前端）

## 常用命令

```bash
make dev                    # 前端 :3000（scripts/dev.py 读锁文件自动拉起/复用后端）
make dev-web / dev-backend  # 单独启动（dev-web 同 dev）
make build                  # 全部平台构建（等价 python3 scripts/build.py）
make build-linux / build-windows / build-darwin / build-darwin-amd64  # 指定平台交叉编译
python3 scripts/build.py windows darwin   # 脚本直接调用，可多平台
python3 scripts/build.py --list / --clean # 列出平台 / 清理产物
make pack                   # 打包安装包（等价 python3 scripts/build.py pack）
make pack-windows / pack-linux            # 指定平台安装包（msi、deb/pacman/AppImage）
```

打包依赖系统工具：wixl（msitools，Windows .msi）、dpkg-deb（dpkg）、makepkg（base-devel）、mksquashfs（squashfs-tools，AppImage 手动构建 + type2 runtime 自动缓存）、可选 rsvg-convert（librsvg）；脚本缺失时提示安装命令。安装包输出到 `build/packages/<平台>/`（Linux 包声明依赖 nodejs，前端为 Next.js standalone）。

## 后端约定

- Go 1.27，模块名 `filespace`；依赖：gopsutil（跨平台采集）、zeroconf（mDNS）、yaml.v3（配置）
- 文件可预览性由**内容嗅探**判定（`http.DetectContentType`），`FileInfo.previewable` 供前端隐藏二进制文件的预览按钮，勿改回硬编码扩展名
- 修改依赖后 `go mod tidy`；提交前 `gofmt` + `go vet` + `go build ./...`

## 前端约定

- Next.js 16 + `output: 'standalone'`（生产构建产出自包含服务器 `.next/standalone/`，由 `start.js` 引导启动；**动态路由段无法预渲染**，用查询参数（如 `/folders?folderId=xxx`））
- `/api/*` 反代统一由 `proxy.ts` 处理（Next.js proxy 文件约定，dev 与生产一致）：后端地址取环境变量 `FILESPACE_BACKEND`（默认 http://127.0.0.1:8080），由 `start.js` / `scripts/dev.py` 按后端实际端口设置；**勿**改回 next.config rewrites（静态导出与 proxy 均不支持）
- `start.js` 是前端引导（读锁文件 / 拉起后端 / 设 PORT 与 FILESPACE_BACKEND / require server.js），随构建拷贝进 `web/` 分发目录；pnpm 布局下 standalone trace 会漏 next 运行时依赖，`build.py` 的 `patch_standalone_deps` 负责补拷，勿删除
- 卡片组件在 `web/app/_cards/`，页面在 `web/app/`，API 封装在 `web/app/_lib/api.ts`
- 预览渲染路由用 `@smazeeapps/file-viewer` 的 `detectFileType`（勿硬编码扩展名）；不支持的非二进制格式以纯文本显示
- **勿移除** `file_preview.tsx` 顶部的 `window.Prism = {manual: true}`（阻止 prism 自动 highlightAll 破坏代码逐行结构）
- **勿移除** `package.json` 的 `postinstall`（修补 ProLayout 内部 Drawer 废弃 width 警告）
- 主题：`AppTheme` 在 `<html>` 上开关 `.dark` class，Tailwind `dark:` 变体为 class 驱动（`@custom-variant dark`）
- 提交前 `tsc --noEmit` + `eslint` + `next build`

## 文档与提交

- 注释、文档、提交信息使用中文
- 更新 AGENTS.md / README.md 后一并提交
- 版本号约定：`backend/version.go` 当前为 `0.1.14`；每次提交代码时先将版本号最后一位（patch）加一，除非用户另有说明
- git 推送必须用 HTTPS remote + gh 凭据助手（本环境 SSH 推送会因 ssh_config.d 权限失败）

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
