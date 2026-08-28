import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

const nextConfig: NextConfig = {
  // 生产构建：静态导出（pnpm build → web/out/，由后端读取托管）
  // 开发模式：普通 dev server，rewrites 代理 /api/* 到后端 :8080
  ...(isDev ? {} : { output: "export" as const }),
  images: { unoptimized: true },
  ...(isDev
    ? {
        async rewrites() {
          return [
            {
              source: "/api/:path*",
              destination: "http://localhost:8080/api/:path*",
            },
          ];
        },
      }
    : {}),
};

export default nextConfig;
