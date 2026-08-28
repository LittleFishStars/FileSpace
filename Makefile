# 文件空间 FileSpace 编译入口
#
# 开发：      make dev             → 同时启动前端(:3000) + 后端(:8080)
#             make dev-web / make dev-backend → 单独启动
# 生产构建：  make build           → 产物统一输出到 build/
# 交叉编译：  make build-windows / build-darwin / build-darwin-amd64
# 清理：      make clean

.PHONY: dev dev-web dev-backend build build-web build-windows build-darwin build-darwin-amd64 clean

# 开发：前端（Next.js dev server，:3000，/api/* 自动代理到 :8080）
dev-web:
	cd web && pnpm dev

# 开发：后端（Go API，:8080）
dev-backend:
	cd backend && go run ./cmd/filespace

# 开发：同时启动前后端（Ctrl+C 退出时自动清理后端进程）
dev:
	@echo "==> 启动后端 :8080（go run）..." ; \
	(cd backend && go run ./cmd/filespace) & \
	BACKEND_PID=$$! ; \
	trap 'kill $$BACKEND_PID 2>/dev/null' EXIT INT TERM ; \
	echo "==> 启动前端 :3000（pnpm dev）..." ; \
	cd web && pnpm dev

# 构建前端静态资源到 build/web/（供 build 与交叉编译目标复用）
build-web:
	mkdir -p build/web
	cd web && pnpm build
	cp -r web/out/. build/web/

# 生产构建：前端静态导出 + Go 二进制（当前平台）
build: build-web
	cd backend && go build -o ../build/filespace ./cmd/filespace
	@echo ""
	@echo "✅ 构建完成:"
	@echo "   二进制: build/filespace"
	@echo "   前端:   build/web/"
	@echo "   运行:   cd build && ./filespace"

# 交叉编译：Windows amd64
build-windows: build-web
	cd backend && GOOS=windows GOARCH=amd64 go build -o ../build/filespace.exe ./cmd/filespace
	@echo "✅ build/filespace.exe（Windows amd64）"

# 交叉编译：macOS（Apple Silicon）
build-darwin: build-web
	cd backend && GOOS=darwin GOARCH=arm64 go build -o ../build/filespace-darwin ./cmd/filespace
	@echo "✅ build/filespace-darwin（macOS arm64）"

# 交叉编译：macOS（Intel）
build-darwin-amd64: build-web
	cd backend && GOOS=darwin GOARCH=amd64 go build -o ../build/filespace-darwin-amd64 ./cmd/filespace
	@echo "✅ build/filespace-darwin-amd64（macOS amd64）"

clean:
	rm -rf build web/out
	@echo "已清理 build/ 与 web/out/"
