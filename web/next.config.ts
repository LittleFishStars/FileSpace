import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

// 开发模式下 /api/* 的代理目标：默认 8080，可用 FILESPACE_BACKEND 覆盖
// （scripts/dev.py 启动前端时会按后端实际端口设置该变量）
const apiTarget = process.env.FILESPACE_BACKEND ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  // 生产构建：静态导出（pnpm build → web/out/，由前端程序 filespace-web 托管）
  // 开发模式：普通 dev server，rewrites 代理 /api/* 到后端（端口见 FILESPACE_BACKEND）
  ...(isDev ? {} : { output: "export" as const }),
  images: { unoptimized: true },
  ...(isDev
    ? {
        async rewrites() {
          return [
            {
              source: "/api/:path*",
              destination: `${apiTarget}/api/:path*`,
            },
          ];
        },
      }
    : {}),
};

export default nextConfig;
