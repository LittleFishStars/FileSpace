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
├── web/                  # 前端（Next.js + antd）
│   ├── app/
│   │   ├── _cards/       # 卡片组件（节点 / 文件夹 / 节点信息）
│   │   └── _components/  # 布局外壳、主题管理
│   └── public/
└── backend/              # 后端（Go）
    ├── cmd/filespace/    # CLI 入口
    ├── internal/
    │   ├── api/          # HTTP API 路由与处理
    │   ├── discovery/    # mDNS 服务注册与发现
    │   ├── share/        # 共享目录扫描 / 索引 / 监听
    │   ├── monitor/      # 系统状态采集（CPU / 内存 / 运行时间）
    │   └── model/        # 数据模型（与前端对齐）
    ├── config.go         # 配置结构体
    ├── config.yaml       # 配置示例
    ├── version.go        # 版本号
    └── web/              # 前端构建产物（go:embed 内嵌，当前为占位）
```

## 架构

P2P + mDNS：每个节点运行一份后端，既对外提供共享，又通过 mDNS 自动发现其他节点；浏览器连到任意一个节点即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点完成。

## 使用

### 后端（每个需要共享的机器）

```bash
# 开发运行：在要共享的文件夹下执行
cd ~/my-shared-folder
go run ./cmd/filespace

# 构建为单二进制
go build ./cmd/filespace
./filespace ~/Documents/public
```

配置见 `backend/config.yaml`：监听端口、共享目录列表、mDNS 服务名、状态采集间隔等。

### 前端

```bash
cd web
pnpm install
pnpm dev
```

浏览器打开 `http://localhost:3000`（生产环境将由后端 `go:embed` 内嵌托管，无需单独部署前端）。

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
- [ ] 后端：共享目录扫描 / 索引 / 文件变更监听
- [ ] 后端：系统状态采集
- [ ] 后端：HTTP API
- [ ] 后端：mDNS 服务注册与发现
- [ ] 前后端联调
