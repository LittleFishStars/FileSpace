<div align="center">

<img src="web/public/filespace-icon.svg" alt="FileSpace" width="120" height="120">

# 文件空间 FileSpace

局域网文件共享工具：在任意文件夹执行 `filespace --web` 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

</div>

## ✨ 能干什么

- **🖥️ 零配置共享**：`cd 某文件夹 && filespace --web` 一键共享当前目录，自动在浏览器中打开界面；未指定目录时自动恢复上次退出前共享的目录
- **📡 mDNS 自动发现**：局域网内节点自动互相发现，无需手动配置地址
- **🌐 Web 界面**：浏览器查看局域网内其他节点及其共享文件夹，支持文件浏览、下载与在线预览（PDF / Office / 代码 / 图片 / 文本，二进制回退下载）；本机共享文件夹在独立的「本机管理」页中添加 / 删除，本机文件用系统默认应用直接打开
- **🖧 P2P 架构**：无中心服务器，节点对等，断网可用
- **🌗 深浅主题**：跟随系统配色，可手动切换（跟随系统 / 浅色 / 深色）

## 🛠 技术栈

| 侧 | 技术 |
|---|---|
| 后端 | Go 1.27 + zeroconf（mDNS）+ gopsutil（跨平台采集）+ yaml.v3 |
| 前端 | Next.js 16（App Router + output: export）+ antd + Tailwind v4 |
| 构建 | Python（`scripts/build.py`，前端静态导出 + go:embed 嵌入后端，按平台交叉编译） |

> 前后端合并为一个程序：**filespace**（Go 后端二进制）。不带参数只启动后端 API；带 `--web` 参数时后端托管前端静态资源（`output: 'export'` → `web/out/`，由 `go:embed` 嵌入），在浏览器中打开界面。没有 Electron，没有 Node 后端。

## 🏗 仓库结构

```text
.
├── Makefile              # 编译入口（构建委托给 scripts/build.py）
├── scripts/              # 构建脚本
│   ├── build.py          # 前端静态导出 + go:embed + 按平台编译后端 + 打包安装包
│   ├── dev.py            # 开发启动：前端 + 后端
│   └── assets/           # 安装包图标源（scripts/assets/filespace.svg）
├── web/                  # 前端（Next.js + antd + Tailwind v4）
│   ├── app/              # 页面（_cards/ 卡片组件、_components/ 布局外壳与主题、_lib/ API 封装）
│   ├── public/           # 静态资源（图标等）
│   └── next.config.ts    # output: export（生产）+ rewrites（开发模式反代 /api）
├── backend/              # 后端（Go）
│   ├── cmd/filespace/    # CLI 入口（flags.go / main.go / run.go / web.go）
│   │   └── web/          # go:embed 嵌入源（构建脚本从 web/out/ 拷贝而来）
│   ├── internal/
│   │   ├── api/          # HTTP API 路由与处理
│   │   ├── config/       # 配置结构体与加载
│   │   ├── discovery/    # mDNS 服务注册与发现
│   │   ├── model/        # 数据模型（与前端对齐）
│   │   ├── monitor/      # 系统状态采集（主机名 / 系统 / 运行时间 / IP，跨平台）
│   │   ├── share/        # 共享目录扫描 / 索引
│   │   └── state/        # 本地状态（上次共享记录、运行锁）
│   ├── config.yaml       # 配置示例
│   └── version.go        # 版本号（当前 0.2.0）
└── build/                # 构建产物（gitignored）：build/<平台>/（后端，含嵌入的前端静态资源）
```

## 💻 支持平台

| 平台 | 架构 | 后端（含前端静态资源） |
|---|---|---|
| Linux | x64 / ARM64 | ✅ |
| Windows | x64 | ✅ |
| macOS | ARM64（Apple Silicon）/ x64（Intel） | ✅ |

## 📦 安装与启动

### 一键启动（开发）

```bash
make dev              # 前端 :3000 + 后端 :8080（scripts/dev.py 自动拉起后端）
make dev-web          # 只启动前端
make dev-backend      # 只启动后端（Go API，:8080）
```

浏览器打开 `http://localhost:3000`。

### 从源码构建

构建由 `scripts/build.py` 完成：前端静态导出（`output: 'export'` → `web/out/`）→ 拷贝到 `backend/cmd/filespace/web/`（go:embed 源）→ 后端按平台交叉编译（前端静态资源嵌入二进制）。

```bash
make build                    # 全部平台（等价 python3 scripts/build.py）
make build-linux              # 只编译 Linux x64
make build-windows            # 只编译 Windows x64
make build-darwin             # 只编译 macOS Apple Silicon
make build-darwin-amd64       # 只编译 macOS Intel
# 直接调用脚本，支持一次指定多个平台
python3 scripts/build.py windows darwin
python3 scripts/build.py --list       # 列出支持的平台
python3 scripts/build.py --clean      # 清理构建产物
```

构建完成后启动：

```bash
# 只启动后端 API
./build/linux/filespace

# 启动后端 + 前端界面，并在浏览器中打开
./build/linux/filespace --web
```

### 打包安装包

`pack` 自动确保先构建产物，输出到 `build/packages/<平台>/`：

```bash
make pack                     # Windows .msi + Linux deb/pacman/AppImage
make pack-windows             # build/packages/windows/FileSpace-<版本>.msi
make pack-linux               # build/packages/linux/ 下 deb + pacman + AppImage
# 等价直接调用脚本
python3 scripts/build.py pack windows
python3 scripts/build.py pack linux
```

| 平台 | 格式 | 依赖工具 |
|---|---|---|
| Windows | `.msi` | `wixl`（`sudo pacman -S msitools`） |
| Linux | `.deb` | `dpkg-deb`（`sudo pacman -S dpkg`） |
| Linux | `.pkg.tar.zst` | `makepkg`（`sudo pacman -S base-devel`） |
| Linux | `.AppImage` | `mksquashfs`（`sudo pacman -S squashfs-tools`）+ type2 runtime（首次自动缓存到 `build/tools/appimage-cache/`） |

> 桌面入口文件的 `Exec` 为 `filespace --web`，AppImage 的 `AppRun` 执行 `filespace --web`。应用图标由 `scripts/assets/filespace.svg` 生成（需要 `librsvg`）。

## 🧩 工作原理

前后端合并为一个程序，运行入口是 `filespace` 后端：

```text
用户执行 filespace --web（共享当前目录）
  ├─ 启动后端 API 服务（默认 :8080）
  ├─ 托管前端静态资源（go:embed 嵌入，/api/* 同源访问）
  └─ 在浏览器中打开 http://localhost:8080
```

不带 `--web` 参数时只启动后端 API，供其他节点或前端访问。

后端（`filespace`）提供 API：共享目录扫描 / 文件浏览下载 / 节点状态，并注册 mDNS 服务、发现其他节点。浏览器连到任意一个节点的 `--web` 界面即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点后端完成（后端保留 CORS 头）。

## ⚙️ 配置与运行锁

配置见 `backend/config.yaml`：监听端口、共享目录列表、mDNS 服务名、状态采集间隔等。

**共享目录恢复**：未指定共享目录（命令行与配置文件均未设置）时，后端会恢复**上次退出前共享的目录**；每次正常退出（Ctrl+C / kill / 终端关闭）前会把当前共享目录记录到用户配置目录下的 `filespace/last-shared.yaml`（Linux `~/.config/filespace/`、macOS `~/Library/Application Support/filespace/`、Windows `%AppData%\filespace\`）。删除该文件即可回到「默认共享当前目录」的行为；`filespace -a` 可在解析出的共享目录之外**额外**共享当前目录（已在列表中则跳过），可与目录参数、配置文件同时使用；`filespace --this` 则**仅**共享当前目录（不恢复上次记录，与目录参数、配置文件中的 shared_folders 互斥）。

**运行锁**：已有一个 `filespace` 后端在运行时（无论端口，通过用户配置目录下的运行锁文件 `filespace/lock` 标识，文件内容为运行中后端的端口），再启动的后端进程检测到锁文件存在且内容端口存活，则仅允许用**目录参数或 `-a`** 把目录追加给已运行的后端（通过 `POST /api/folders/add` 交给它，该接口仅允许本机回环地址调用），自身随即退出；无追加内容（如无参数或 `--this`）时提示并退出。后端正常退出会删除锁文件；进程崩溃残留的锁文件会在下次启动时被自动清理重建。

## 📡 API 一览

| 接口 | 说明 |
|---|---|
| `GET /api/node` | 本节点信息 |
| `GET /api/folders` | 本节点共享的文件夹列表 |
| `POST /api/folders/add` | 追加共享目录（仅供本机回环地址调用，同机另一 filespace 进程移交目录用） |
| `POST /api/folders/remove` | 移除共享目录（仅供本机回环地址调用，本机管理页用） |
| `GET /api/folders/{id}/tree` | 文件树（懒加载） |
| `GET /api/folders/{id}/download` | 文件下载（支持 Range 断点续传） |
| `POST /api/folders/{id}/open` | 用系统默认应用打开本机文件（仅供本机回环地址调用，xdg-open / open / cmd start） |
| `GET /api/peers` | mDNS 发现的其他节点（含其共享文件夹） |
