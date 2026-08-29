# 文件空间 FileSpace 编译入口
#
# 开发：      make dev             → 前端（:3000）+ 后端（:8080，自动拉起）
#             make dev-web / make dev-backend → 单独启动
# 生产构建：  make build           → 全部平台（等价 python3 scripts/build.py）
# 指定平台：  make build-linux / build-windows / build-darwin / build-darwin-amd64
# 打包：      make pack            → Windows/Linux 安装包（等价 python3 scripts/build.py pack）
#             make pack-windows / make pack-linux → 指定平台安装包
# 清理：      make clean
#
# 构建/打包逻辑统一由 scripts/build.py（Python）实现，产物输出到 build/ 下：
#   build/<平台>/               后端（filespace，含嵌入的前端静态资源）
# 运行：
#   build/<平台>/filespace            # 只启动后端 API
#   build/<平台>/filespace --web      # 启动后端 + 前端界面，并在浏览器中打开
# 安装包输出到 build/packages/<平台>/。

.PHONY: dev dev-web dev-backend build build-linux build-windows build-darwin build-darwin-amd64 pack pack-windows pack-linux clean

# 开发：前端（Next.js dev server，:3000）+ 后端（Go API，:8080）
# （scripts/dev.py 自动拉起后端，前端通过 rewrites 反代 /api）
dev-web:
	python3 scripts/dev.py

# 开发：后端（Go API，:8080；如需指定端口：go run ./cmd/filespace -p 9000）
dev-backend:
	cd backend && go run ./cmd/filespace

# 开发：一条命令（等价 dev-web：前端 + 后端）
dev:
	python3 scripts/dev.py

# 生产构建：全部平台（linux / windows / darwin / darwin-amd64）
build:
	python3 scripts/build.py

# 生产构建：指定平台
build-linux:
	python3 scripts/build.py linux

build-windows:
	python3 scripts/build.py windows

build-darwin:
	python3 scripts/build.py darwin

build-darwin-amd64:
	python3 scripts/build.py darwin-amd64

# 打包安装包：全部（windows + linux）/ 指定平台
pack:
	python3 scripts/build.py pack

pack-windows:
	python3 scripts/build.py pack windows

pack-linux:
	python3 scripts/build.py pack linux

clean:
	python3 scripts/build.py --clean
