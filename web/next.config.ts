import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

const nextConfig: NextConfig = {
  // 生产构建：standalone 自包含服务器（pnpm build → .next/standalone/，前端独立运行）。
  // 由 web/start.js 引导：读取后端锁文件、必要时拉起后端，并通过 FILESPACE_BACKEND
  // 环境变量告知后端地址；/api/* 反代统一由 proxy.ts 处理（dev 与生产一致）。
  ...(isDev ? {} : { output: "standalone" as const }),
  images: { unoptimized: true },
};

export default nextConfig;
