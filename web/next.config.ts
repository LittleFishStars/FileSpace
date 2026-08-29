import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

// 生产构建：静态导出（output: 'export'），由 filespace 后端直接托管前端静态资源。
// 开发模式：Next.js dev server，用 rewrites 把 /api 反代到后端（:8080）。
const nextConfig: NextConfig = {
  images: { unoptimized: true },
};

if (isDev) {
  nextConfig.rewrites = async () => [
    { source: "/api/:path*", destination: "http://127.0.0.1:8080/api/:path*" },
  ];
} else {
  nextConfig.output = "export";
}

export default nextConfig;
