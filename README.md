# 文件空间 FileSpace

局域网文件共享工具：在任意文件夹下执行 `filespace` 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

## 特性

- 🖥️ 零配置共享：`cd 某文件夹 && filespace` 一键共享当前目录；未指定目录时自动恢复上次退出前共享的目录（`filespace -a` 可强制共享当前目录），也可指定共享目录/文件
- 📡 mDNS 自动发现：局域网内节点自动互相发现，无需手动配置地址
- 🌐 Web 界面：浏览器查看所有节点及其共享文件夹，支持文件浏览与下载
- 🖧 P2P 架构：无中心服务器，节点对等，断网可用
- 🌗 深浅主题：跟随系统配色，可手动切换（跟随系统 / 浅色 / 深色）

## 目录结构

```text
.
├── Makefile              # 编译入口（构建委托给 scripts/build.py）
├── scripts/              # 构建脚本
│   └── build.py          # Python 构建脚本（按平台编译，产物输出到 build/<平台>/）
├── web/                  # 前端（Next.js + antd）
│   ├── app/
│   │   ├── _cards/       # 卡片组件（节点 / 文件夹 / 节点信息）
│   │   └── _components/  # 布局外壳、主题管理
│   └── public/
├── backend/              # 后端（Go）
│   ├── cmd/filespace/    # CLI 入口
│   ├── internal/
│   │   ├── api/          # HTTP API 路由与处理
│   │   ├── discovery/    # mDNS 服务注册与发现
│   │   ├── share/        # 共享目录扫描 / 索引
│   │   ├── monitor/      # 系统状态采集（主机名 / 系统 / 运行时间 / IP，跨平台）
│   │   └── model/        # 数据模型（与前端对齐）
│   ├── config.go         # 配置结构体
│   ├── config.yaml       # 配置示例
│   └── version.go        # 版本号
└── build/                # 构建产物（gitignored）：build/<平台>/ 下为二进制 + web/ 静态资源
```

## 架构

P2P + mDNS：每个节点运行一份后端，既对外提供共享，又通过 mDNS 自动发现其他节点；浏览器连到任意一个节点即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点完成。

## 使用

### 一键构建（前后端统一输出到 build/<平台>/）

构建由 `scripts/build.py` 完成：先构建平台无关的前端静态资源，再按平台交叉编译后端，每个平台目录都是一个完整可分发单元（二进制 + `web/`）。

```bash
make build                 # 编译全部平台（等价 python3 scripts/build.py）
make build-linux           # 只编译 Linux x86_64
make build-windows         # 只编译 Windows amd64
make build-darwin          # 只编译 macOS Apple Silicon
make build-darwin-amd64    # 只编译 macOS Intel
# 产物：
#   build/linux/filespace.exe 等 → build/<平台>/filespace（Windows 为 filespace.exe）
#   build/<平台>/web/           → 前端静态资源
cd build/linux && ./filespace
```

也可直接调用脚本，支持一次指定多个平台：

```bash
python3 scripts/build.py              # 全部平台
python3 scripts/build.py windows      # 指定平台
python3 scripts/build.py windows darwin  # 多个平台
python3 scripts/build.py --list       # 列出支持的平台
python3 scripts/build.py --clean      # 清理构建产物
```

`build/<平台>/filespace` 运行时自动定位同级 `web/` 目录（找不到时回退到当前目录下的 `web/`，deb / pacman 安装后还会查找系统目录 `/usr/share/filespace/web`）。

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

> deb / pacman 包安装到 `/usr/bin/filespace`，前端资源在 `/usr/share/filespace/web`；AppImage 内含完整资源（二进制同级 `web/`），保持用户当前工作目录共享。应用图标由 `scripts/assets/filespace.svg` 生成（需要 `librsvg`）。

### 交叉编译（后端为纯 Go，可跨平台构建）

支持 **Linux / macOS / Windows**（含运行时间与系统名读取的平台实现）；前端 `build/<平台>/web/` 与平台无关，随对应平台目录一同分发。

### 开发

```bash
# 一条命令同时启动前后端
make dev

# 或分别启动（适合 IDE 里分开运行）
make dev-web       # 前端（:3000，/api/* 自动代理到 :8080）
make dev-backend   # 后端（:8080）
```

浏览器打开 `http://localhost:3000`；生产环境前后端同源（后端托管 `build/web/`），无需单独部署前端。

配置见 `backend/config.yaml`：监听端口、共享目录列表、mDNS 服务名、状态采集间隔等。

未指定共享目录（命令行与配置文件均未设置）时，后端会恢复**上次退出前共享的目录**；每次正常退出（Ctrl+C / kill / 终端关闭）前会把当前共享目录记录到用户配置目录下的 `filespace/last-shared.yaml`（Linux `~/.config/filespace/`、macOS `~/Library/Application Support/filespace/`、Windows `%AppData%\filespace\`）。删除该文件即可回到「默认共享当前目录」的行为；不加目录参数只想直接共享当前文件夹时，用 `filespace -a`（`-a` 与目录参数、配置文件中的 shared_folders 互斥）。

## API 一览

| 接口 | 说明 |
|---|---|
| `GET /api/node` | 本节点信息 |
| `GET /api/folders` | 本节点共享的文件夹列表 |
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
- [x] 前后端联调
- [ ] 后端：文件变更实时监听（fsnotify）
