# 文件空间 FileSpace

局域网文件共享工具：在任意文件夹下执行前端程序即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

前后端是两个独立程序：**filespace**（后端，Go，P2P + mDNS + 文件共享 API）与 **前端**（Next.js standalone 服务器，即前端本身，无额外启动器程序）。

## 特性

- 🖥️ 零配置共享：`cd 某文件夹 && node web/start.js`（安装包为 `filespace-web`）一键共享当前目录（前端自动拉起后端）；未指定目录时自动恢复上次退出前共享的目录（`filespace -a` 可在恢复之外额外共享当前目录，`filespace --this` 仅共享当前目录），也可指定共享目录/文件
- 🔌 前后端分离：前端（Next.js）启动时读取后端锁文件获取端口；后端未启动则自动拉起一个（默认共享当前目录），退出时一并清理；后端已运行则自动复用其端口
- 📡 mDNS 自动发现：局域网内节点自动互相发现，无需手动配置地址
- 🌐 Web 界面：浏览器查看所有节点及其共享文件夹，支持文件浏览与下载
- 🖧 P2P 架构：无中心服务器，节点对等，断网可用
- 🌗 深浅主题：跟随系统配色，可手动切换（跟随系统 / 浅色 / 深色）

## 目录结构

```text
.
├── Makefile              # 编译入口（构建委托给 scripts/build.py）
├── scripts/              # 构建脚本
│   ├── build.py          # Python 构建脚本（按平台编译，产物输出到 build/<平台>/）
│   └── dev.py            # 开发启动：前端自动拉起/复用后端
├── web/                  # 前端（Next.js + antd）
│   ├── app/              # 页面（_cards/ 卡片组件、_components/ 布局外壳与主题）
│   ├── public/
│   ├── proxy.ts          # /api/* 反代到后端（Next.js proxy 文件约定，dev 与生产一致）
│   └── start.js          # 前端引导：读锁文件 / 拉起后端 / 启动 standalone 服务器
├── backend/              # 后端（Go）
│   ├── cmd/filespace/    # CLI 入口（参数解析 / 共享目录解析 / 移交已有后端 / 服务运行）
│   ├── internal/
│   │   ├── api/          # HTTP API 路由与处理
│   │   ├── config/       # 配置结构体与加载
│   │   ├── discovery/    # mDNS 服务注册与发现
│   │   ├── model/        # 数据模型（与前端对齐）
│   │   ├── monitor/      # 系统状态采集（主机名 / 系统 / 运行时间 / IP，跨平台）
│   │   ├── share/        # 共享目录扫描 / 索引
│   │   └── state/        # 本地状态（上次共享记录、运行锁）
│   ├── config.yaml       # 配置示例
│   └── version.go        # 版本号
└── build/                # 构建产物（gitignored）：build/<平台>/ 下为后端二进制 + web/ 前端
```

## 架构

两个独立程序，运行入口是前端（Next.js standalone 服务器）：

```text
用户执行 node web/start.js（共享当前目录）
  ├─ 读取锁文件（用户配置目录 filespace/lock）→ 得到后端端口
  ├─ 后端未启动？→ 自动拉起 filespace -p <端口>（工作目录 = 当前目录），轮询等待就绪
  ├─ 监听前端端口（默认 8080，被占用时自动顺延），启动 Next.js standalone 服务器
  └─ /api/* 请求由 proxy.ts 反向代理给后端（浏览器同源访问，无需 CORS）
```

后端（`filespace`）只提供 API：共享目录扫描 / 文件浏览下载 / 节点状态，并注册 mDNS 服务、发现其他节点。浏览器连到任意一个节点的前端即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点后端完成（后端保留 CORS 头）。

前端退出时：若后端由它拉起，会通知后端优雅退出（记录共享目录、删除锁文件）；若复用已有后端，则不影响其生命周期。

## 使用

### 依赖

- 后端：纯 Go 二进制（跨平台）。
- 前端：Next.js standalone 服务器，运行需要 **Node.js 18.18+**（仅运行产物，无需 `pnpm install`，构建时已内置最小 `node_modules`）。

### 一键构建（前后端统一输出到 build/<平台>/）

构建由 `scripts/build.py` 完成：先构建 Next.js standalone 前端（`.next/standalone` + 静态资源 + `start.js` 组装为 `web/` 分发目录），再按平台交叉编译后端（`filespace`）。

```bash
make build                 # 编译全部平台（等价 python3 scripts/build.py）
make build-linux           # 只编译 Linux x86_64
make build-windows         # 只编译 Windows amd64
make build-darwin          # 只编译 macOS Apple Silicon
make build-darwin-amd64    # 只编译 macOS Intel
# 产物：
#   build/<平台>/filespace         → 后端（纯 API）
#   build/<平台>/web/              → Next.js standalone 前端（server.js + node_modules + .next/ + start.js）
cd build/linux && node web/start.js
```

也可直接调用脚本，支持一次指定多个平台：

```bash
python3 scripts/build.py              # 全部平台
python3 scripts/build.py windows      # 指定平台
python3 scripts/build.py windows darwin  # 多个平台
python3 scripts/build.py --list       # 列出支持的平台
python3 scripts/build.py --clean      # 清理构建产物
```

`build/<平台>/web/start.js` 自动定位同级 `filespace` 后端（或 PATH 中的 `filespace`）并启动。

### 打包安装包

`pack` 自动确保先构建产物，输出到 `build/packages/<平台>/`：

```bash
make pack                  # Windows .msi + Linux deb/pacman/AppImage
make pack-windows          # 只打 Windows：build/packages/windows/FileSpace-<版本>.msi
make pack-linux            # 只打 Linux：build/packages/linux/ 下 deb + pacman + AppImage
# 等价直接调用脚本：
python3 scripts/build.py pack            # 全部
python3 scripts/build.py pack windows    # Windows
python3 scripts/build.py pack linux      # Linux
```

| 平台 | 格式 | 依赖工具（缺失时脚本会提示安装命令） |
|---|---|---|
| Windows | `.msi` | `wixl`（`sudo pacman -S msitools`） |
| Linux | `.deb` | `dpkg-deb`（`sudo pacman -S dpkg`） |
| Linux | pacman 包（`.pkg.tar.zst`） | `makepkg`（`sudo pacman -S base-devel`） |
| Linux | `.AppImage` | `mksquashfs`（`sudo pacman -S squashfs-tools`）+ type2 runtime（首次自动下载缓存到 `build/tools/appimage-cache/`） |

> deb / pacman 包安装 `filespace` 到 `/usr/bin/`、前端 `web/` 到 `/usr/share/filespace/web/`，并提供 `/usr/bin/filespace-web` 命令作为前端入口（依赖 `nodejs`，安装包声明 `Depends: nodejs`）；AppImage 内含完整资源（`web/` 与 `filespace` 同级），保持用户当前工作目录共享。应用图标由 `scripts/assets/filespace.svg` 生成（需要 `librsvg`）。

### 交叉编译（后端为纯 Go，可跨平台构建）

支持 **Linux / macOS / Windows**（含运行时间与系统名读取的平台实现）；前端 `build/<平台>/web/` 与平台无关，随对应平台目录一同分发（目标机器需安装 Node.js）。

### 开发

```bash
# 一条命令启动前端（Next.js dev :3000），自动拉起/复用后端
make dev

# 或分别启动（适合 IDE 里分开运行）
make dev-web       # 前端（:3000）：scripts/dev.py 读后端锁文件，未启动则 go run 拉起，退出时清理
make dev-backend   # 后端（Go API，:8080）
```

`scripts/dev.py` 启动前端时会把后端实际端口通过 `FILESPACE_BACKEND` 环境变量交给 next dev，`proxy.ts` 据此反代 `/api/*`（dev 与生产同一套反代逻辑，默认 `http://127.0.0.1:8080`）。

浏览器打开 `http://localhost:3000`；生产环境由 `start.js` 启动 standalone 服务器并反代 `/api` 到后端，前后端同源，无需单独部署。

配置见 `backend/config.yaml`：监听端口、共享目录列表、mDNS 服务名、状态采集间隔等。

未指定共享目录（命令行与配置文件均未设置）时，后端会恢复**上次退出前共享的目录**；每次正常退出（Ctrl+C / kill / 终端关闭）前会把当前共享目录记录到用户配置目录下的 `filespace/last-shared.yaml`（Linux `~/.config/filespace/`、macOS `~/Library/Application Support/filespace/`、Windows `%AppData%\filespace\`）。删除该文件即可回到「默认共享当前目录」的行为；`filespace -a` 可在解析出的共享目录之外**额外**共享当前目录（已在列表中则跳过），可与目录参数、配置文件同时使用；`filespace --this` 则**仅**共享当前目录（不恢复上次记录，与目录参数、配置文件中的 shared_folders 互斥）。

**运行锁与前后端协作**：已有一个 `filespace` 后端在运行时（无论端口，通过用户配置目录下的运行锁文件 `filespace/lock` 标识，文件内容为运行中后端的端口，如 Linux `~/.config/filespace/lock`），再启动的后端进程检测到锁文件存在且内容端口存活，则仅允许用**目录参数或 `-a`** 把目录追加给已运行的后端（通过 `POST /api/folders/add` 交给它，该接口仅允许本机回环地址调用），自身随即退出；无追加内容（如无参数或 `--this`）时提示并退出。后端正常退出会删除锁文件；进程崩溃残留的锁文件会在下次启动时被自动清理重建。前端（`start.js` / `scripts/dev.py`）启动时同样读取该锁文件：端口存活则复用该后端，否则自动拉起一个后端并等待就绪。追加的目录同样会被记录，下次启动时一并恢复；重复追加同一目录（含符号链接指向同一目录）会被自动去重，但允许同时共享父目录与其子目录。

## API 一览

| 接口 | 说明 |
|---|---|
| `GET /api/node` | 本节点信息 |
| `GET /api/folders` | 本节点共享的文件夹列表 |
| `POST /api/folders/add` | 追加共享目录（仅供本机回环地址调用，同机另一 filespace 进程移交目录用） |
| `GET /api/folders/{id}/tree` | 文件树（懒加载） |
| `GET /api/folders/{id}/download` | 文件下载（支持 Range 断点续传） |
| `GET /api/peers` | mDNS 发现的其他节点（含其共享文件夹） |

## 开发状态

- [x] 前端：节点卡片、文件夹卡片、节点信息、深浅主题
- [x] 前端：文件浏览（面包屑 / 文件列表 / 分页）
- [x] 前端：在线预览（PDF 走浏览器原生，Office/代码/文本/图片等走 @smazeeapps/file-viewer，二进制回退下载）
- [x] 后端：共享目录扫描 / 索引
- [x] 后端：系统状态采集（跨平台：Linux / macOS / Windows）
- [x] 后端：HTTP API（节点 / 文件夹 / 文件树 / 下载）
- [x] 后端：mDNS 服务注册与发现
- [x] 前后端分离：前端即 Next.js 本体（standalone 服务器 + start.js 引导 + proxy.ts 反代）
- [x] 前后端联调
- [ ] 后端：文件变更实时监听（fsnotify）
