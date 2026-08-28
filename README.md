# 文件空间 FileSpace

局域网文件共享工具：在任意文件夹下执行 `filespace` 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

## 特性

- 🖥️ 零配置共享：`cd 某文件夹 && filespace` 一键共享当前目录，也可指定共享目录/文件
- 📡 mDNS 自动发现：局域网内节点自动互相发现，无需手动配置地址
- 🌐 Web 界面：浏览器查看所有节点及其共享文件夹，支持文件浏览与下载
- 🖧 P2P 架构：无中心服务器，节点对等，断网可用
- 🌗 深浅主题：跟随系统配色，可手动切换（跟随系统 / 浅色 / 深色）

## 目录结构

```text
.
├── Makefile              # 编译入口
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
└── build/                # 构建产物（gitignored）：filespace 二进制 + web/ 静态资源
```

## 架构

P2P + mDNS：每个节点运行一份后端，既对外提供共享，又通过 mDNS 自动发现其他节点；浏览器连到任意一个节点即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点完成。

## 使用

### 一键构建（前后端统一输出到 build/）

```bash
make build
# 产物：
#   build/filespace   —— Go 二进制
#   build/web/        —— 前端静态资源
cd build && ./filespace
```

`build/filespace` 运行时自动定位同级 `web/` 目录（找不到时回退到当前目录下的 `web/`）。

### 交叉编译（后端为纯 Go，可跨平台构建）

```bash
make build-windows         # Windows amd64 → build/filespace.exe
make build-darwin          # macOS Apple Silicon → build/filespace-darwin
make build-darwin-amd64    # macOS Intel → build/filespace-darwin-amd64
```

后端支持 **Linux / macOS / Windows**（含运行时间与系统名读取的平台实现）；前端 `build/web/` 与平台无关，可直接随二进制分发。

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
