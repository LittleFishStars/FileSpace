# 文件空间 FileSpace 编译入口
#
# 开发：      make dev-web / make dev-backend（两个终端分别运行）
# 生产构建：  make build   → 产物统一输出到 build/
# 清理：      make clean

.PHONY: dev dev-web dev-backend build clean

# 开发：前端（Next.js dev server，:3000，/api/* 自动代理到 :8080）
dev-web:
	cd web && pnpm dev

# 开发：后端（Go API，:8080）
dev-backend:
	cd backend && go run ./cmd/filespace

# 开发：提示同时启动两个服务
dev:
	@echo "请开两个终端分别运行:"
	@echo "  make dev-web      # 前端 :3000"
	@echo "  make dev-backend  # 后端 :8080"
	@echo "（WebStorm 中可建一个 Compound 运行配置同时启动两者）"

# 生产构建：前端静态导出 + Go 二进制，统一输出到 build/
build:
	@echo "==> 构建前端（静态导出）..."
	mkdir -p build/web
	cd web && pnpm build
	cp -r web/out/. build/web/
	@echo "==> 编译后端..."
	cd backend && go build -o ../build/filespace ./cmd/filespace
	@echo ""
	@echo "✅ 构建完成:"
	@echo "   二进制: build/filespace"
	@echo "   前端:   build/web/"
	@echo "   运行:   cd build && ./filespace"

clean:
	rm -rf build web/out
	@echo "已清理 build/ 与 web/out/"
