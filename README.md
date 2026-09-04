<div align="center">

<img src="web/public/filespace-icon.svg" alt="FileSpace" width="120" height="120">

# FileSpace | 文件空间

局域网文件共享工具：用 `-d/--dir` 指定或配置文件列出要共享的文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

</div>

## ✨ 能干什么

- **🖥️ 配置文件驱动共享**：无参数运行自动读取（不存在则创建）用户配置目录下的默认配置文件 `filespace/config.yaml`，按其 `shared_folders` 共享；也可用 `-d/--dir` 临时指定共享文件夹；本机管理页添加/移除的共享会同步写回配置文件
- **📡 mDNS 自动发现**：局域网内节点自动互相发现，无需手动配置地址
- **🌐 Web 界面**：浏览器查看局域网内其他节点及其共享文件夹，支持文件浏览、下载与在线预览（PDF / Office / 代码 / 图片 / 文本，二进制回退下载）；本机共享文件夹在独立的「本机管理」页中管理（系统原生目录选择器或手动输入添加 / 删除），点击文件夹行进入文件浏览页，本机文件也可用系统默认应用直接打开
- **🖧 P2P 架构**：无中心服务器，节点对等，断网可用
- **🌗 深浅主题**：跟随系统配色，可手动切换（跟随系统 / 浅色 / 深色）

## 🛠 技术栈

| 侧 | 技术 |
|---|---|
| 后端 | Go 1.27 + zeroconf（mDNS）+ gopsutil（跨平台采集）+ yaml.v3 |
| 前端 | Next.js 16（App Router + output: export）+ antd + Tailwind v4 |
| 构建 | Python（`scripts/build.py`，前端静态导出 + go:embed 嵌入后端，按平台交叉编译） |

> 前后端合并为一个程序：**filespace**（Go 后端二进制）。无参数运行读取（不存在则创建）默认配置文件并按其共享；带 `--web` 参数时后端托管前端静态资源（`output: 'export'` → `web/out/`，由 `go:embed` 嵌入），在浏览器中打开界面。没有 Electron，没有 Node 后端。

## 🏗 仓库结构

```text
.
├── Makefile              # 编译入口（构建委托给 scripts/build.py）
├── scripts/              # 构建脚本
│   ├── build.py          # 前端静态导出 + go:embed + 按平台编译后端
│   ├── dev.py            # 开发启动：前端 + 后端
│   └── assets/           # Windows exe 图标源（scripts/assets/filespace.svg → .ico）
├── web/                  # 前端（Next.js + antd + Tailwind v4）
│   ├── app/              # 页面（_cards/ 卡片组件、_components/ 布局外壳与主题、_lib/ API 封装）
│   ├── public/           # 静态资源（图标等）
│   └── next.config.ts    # output: export（生产）+ rewrites（开发模式反代 /api）
├── backend/              # 后端（Go）
│   ├── cmd/filespace/    # CLI 入口（flags.go / main.go / run.go / web.go）
│   │   └── web/          # go:embed 嵌入源（构建脚本从 web/out/ 拷贝而来）
│   ├── internal/
│   │   ├── api/          # HTTP API 路由与处理（server 路由装配 / static 托管 / respond 响应助手）
│   │   ├── auth/         # 访问密码哈希与令牌签发校验（share 与 api 共同依赖的单一契约）
│   │   ├── config/       # 配置结构体与加载（默认配置文件创建/写回）
│   │   ├── discovery/    # mDNS 服务注册与发现
│   │   ├── model/        # 数据模型（与前端对齐）
│   │   ├── monitor/      # 系统状态采集（主机名 / 系统 / 运行时间 / IP，跨平台）
│   │   ├── share/        # 共享目录：注册表(manager) / 密码(password) / 统计缓存(stats) / 扫描(tree) / 监听(watcher)
│   │   └── state/        # 本地运行锁
│   ├── config.yaml       # 配置示例
│   └── version.go        # 版本号（当前 0.3.6-2609042009）
└── build/                # 构建产物（gitignored）：build/<平台>/（后端，含嵌入的前端静态资源）
```

## 💻 支持平台

| 平台 | 架构 | 后端（含前端静态资源） |
|---|---|---|
| Linux | x64 / ARM64 | ✅ |
| Windows | x64 | ✅ |
| macOS | ARM64（Apple Silicon）/ x64（Intel） | ✅ |

> ⚠️ **macOS 未经测试**：作者没有 Mac 电脑，macOS（darwin）二进制为交叉编译产物，「在浏览器中打开界面」（`open`）与系统原生目录选择器（`osascript`）等 macOS 特有逻辑未在真实 Mac 上验证，如遇问题请提交 Issue 反馈。

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
# 读取/创建默认配置并按配置共享（不共享任何文件夹时后端仅提供 API）
./build/linux/filespace

# 额外共享指定目录
./build/linux/filespace -d ~/docs

# 启动后端 + 前端界面，并在浏览器中打开
./build/linux/filespace --web -d ~/docs
```

## 🧩 工作原理

前后端合并为一个程序，运行入口是 `filespace` 后端：

```text
用户执行 filespace --web -d ~/docs（按配置共享 + 额外共享 ~/docs）
  ├─ 读取/创建默认配置文件（~/.config/filespace/config.yaml）
  ├─ 启动后端 API 服务（默认 :8080）
  ├─ 托管前端静态资源（go:embed 嵌入，/api/* 同源访问）
  └─ 在浏览器中打开 http://localhost:8080
```

不带 `--web` 参数时只启动后端 API，供其他节点或前端访问。

后端（`filespace`）提供 API：共享目录扫描 / 文件浏览下载 / 节点状态，并注册 mDNS 服务、发现其他节点。浏览器连到任意一个节点的 `--web` 界面即可看到局域网内全部共享内容，文件下载由浏览器直连目标节点后端完成（后端保留 CORS 头）。

## ⚙️ 配置与运行锁

### 配置文件

无参数运行 `filespace` 时，读取用户配置目录下的默认配置文件：

| 平台 | 路径 |
|---|---|
| Linux | `~/.config/filespace/config.yaml` |
| macOS | `~/Library/Application Support/filespace/config.yaml` |
| Windows | `%AppData%\filespace\config.yaml` |

文件不存在则自动创建带注释的模板（含 `shared_folders: []`，不共享任何文件夹）。可用 `-c, --config <文件>` 指定其它配置文件。仓库内 `backend/config.yaml` 为配置示例（监听端口、共享目录列表、默认访问密码、mDNS 服务名等）。

**共享目录持久化**：启动时按配置文件的 `shared_folders` 共享；`-d/--dir` 指定的目录临时追加到本次运行（不写入文件）。**本机管理页**（`/local`）添加/移除共享、设置/修改/移除访问密码时，会立即把当前共享列表写回当前生效的配置文件（`shared_folders` 只存密码的 sha256 哈希，不存明文），保证重启后列表与密码不丢失。`-c` 显式指定的配置文件同样会被写回。

### 运行锁

已有一个 `filespace` 后端在运行时（无论端口，通过用户配置目录下的运行锁文件 `filespace/lock` 标识，文件内容为运行中后端的端口），再启动的后端进程检测到锁文件存在且内容端口存活，则仅允许用 `-d/--dir` 把目录追加给已运行的后端（通过 `POST /api/folders/add` 交给它，该接口仅允许本机回环地址调用），自身随即退出；无追加内容（如无参数）时提示并退出。后端正常退出会删除锁文件；进程崩溃残留的锁文件会在下次启动时被自动清理重建。

### 共享访问密码

每个共享文件夹可单独设置访问密码。
- **命令行**：`-P, --passwd <密码>`（或配置文件顶层 `passwd`）作为**默认密码**，应用于本次共享的所有文件夹（配置文件中为单个文件夹显式设置的 `shared_folders[].passwd` 优先，不被覆盖），如 `filespace -P secret -d ~/docs`。
- **web 端**：本机管理页「添加共享文件夹」时可为所选文件夹设置独立访问密码（也可留空不设）。
- 设置后，**其他节点**需输入该文件夹的密码才能查看/下载其内容：前端进入该文件夹时弹出密码输入框，向后端 `POST /api/auth` 换取访问令牌（有效期 24 小时，过期后需重新输入），文件树/下载请求携带该令牌；同一密码的多个文件夹可共用同一令牌。密码与令牌均不通过 mDNS / 元数据接口传输，仅 `/api/node` 与 `/api/folders` 返回 `auth` 标记供前端显示锁图标。
- **本机不受影响**（本机回环访问自动豁免，`/local` 管理页与「打开」操作照常）。未设置密码的文件夹行为与原来一致（局域网开放）。

## 🔧 命令行参数

```
filespace — 文件空间后端（局域网文件共享 API + 前端托管）

用法:
  filespace [选项]

参数:
  -d, --dir <目录>        要共享的文件夹，可多次指定（在配置文件 shared_folders 之外追加）
  --web                   同时启动前端界面并在浏览器中打开（默认只启动后端 API）
  -c, --config <文件>     配置文件路径（YAML，默认 <用户配置目录>/filespace/config.yaml）
  -p, --port <端口>       监听端口（默认 8080）
  -P, --passwd <密码>     访问密码。两种情形：
                           · 本进程作为后端启动时：作为默认密码，应用于本次共享的文件夹
                             （未显式设置密码的），其他节点需输入密码才能查看/下载；本机不受影响
                           · 已有后端在运行时：需与 -d/--dir 配合，修改该目录的访问密码
                             （传空值 -P '' 表示移除密码）；目录未共享时按「新增共享并设密码」处理
                           也可在 web 端添加/管理共享时按文件夹单独设置密码
  -h, --help              显示本帮助信息

示例:
  filespace                        读取/创建默认配置文件并按配置共享（无共享目录则不共享）
  filespace --web                  启动后端 + 前端界面，并在浏览器中打开
  filespace --web -p 9000          指定端口并启动前端
  filespace -d ~/docs              按默认配置共享，并额外共享 ~/docs
  filespace -d ~/docs -d /mnt/data 额外共享多个目录
  filespace -c config.yaml         使用指定配置文件
  filespace -P secret -d ~/docs    设置共享访问密码，并共享 ~/docs
  filespace -d ~/docs -P newpass   已有后端运行时：修改共享目录 ~/docs 的访问密码
  filespace -d ~/docs -P ''        已有后端运行时：移除 ~/docs 的访问密码
```

配置优先级：命令行 `-p` > 配置文件 > 默认值；`-d/--dir` 指定的目录追加到配置文件的 `shared_folders` 之后（重复路径自动去重）。无参数且配置未列共享目录时，后端启动但不共享任何文件夹（仅提供 API，可在本机管理页添加）。

## 📡 API 一览

| 接口 | 说明 |
|---|---|
| `GET /api/node` | 本节点信息（含 `auth` 标记：本节点是否有需要密码的共享文件夹） |
| `GET /api/folders` | 本节点共享的文件夹列表（每个文件夹含 `auth` 标记：是否设置了访问密码；文件数 / 总大小 / 最近更新由后台缓存扫描提供，目录较大时首次加载可能有短暂延迟） |
| `POST /api/auth` | 校验共享访问密码，签发访问令牌（令牌绑定密码，同一密码的文件夹可共用；本节点没有需要密码的文件夹时返回 404） |
| `POST /api/folders/add` | 追加共享目录（仅供本机回环地址调用，同机另一 filespace 进程移交目录用；本机管理页添加时可用 `password` 字段为该文件夹设置访问密码） |
| `POST /api/folders/remove` | 移除共享目录（仅供本机回环地址调用，本机管理页用） |
| `POST /api/local/pick-directory` | 在本机弹出系统原生目录选择对话框并返回所选目录绝对路径（仅供本机回环地址调用；Linux 用 zenity/kdialog 等、Windows 用 PowerShell、macOS 用 osascript；用户取消返回 `cancelled`） |
| `GET /api/folders/{id}/tree` | 文件树（懒加载；文件夹设置密码时需携带访问令牌，本机回环豁免） |
| `GET /api/folders/{id}/download` | 文件下载（支持 Range 断点续传；文件夹设置密码时需携带访问令牌，本机回环豁免） |
| `POST /api/folders/{id}/open` | 用系统默认应用打开本机文件（仅供本机回环地址调用，xdg-open / open / cmd start） |
| `GET /api/peers` | mDNS 发现的其他节点（含其共享文件夹） |
