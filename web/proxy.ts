// proxy.ts —— 文件空间前端反代：把 /api/* 请求转发给后端（filespace）。
//
// 后端地址由启动器（start.js / scripts/dev.py）经环境变量 FILESPACE_BACKEND 传入，
// 默认 http://127.0.0.1:8080。浏览器侧始终使用同源相对路径 /api，
// 前后端分离后无需改动任何页面代码。
//
// 说明：本文件是 Next.js 的 proxy 文件约定（原 middleware），在请求到达路由前执行，
// 仅支持 Node.js 运行时（Next 16 默认），静态导出（output: export）不支持 proxy，
// 因此生产模式使用 output: 'standalone'。

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export async function proxy(request: NextRequest) {
  const backend =
    process.env.FILESPACE_BACKEND ?? "http://127.0.0.1:8080";

  const { pathname, search } = request.nextUrl;
  const target = `${backend}${pathname}${search}`;

  // 转发请求：保留方法 / 请求头（含 Range 断点续传）/ 请求体
  const headers = new Headers(request.headers);
  headers.delete("host");
  const init: RequestInit = {
    method: request.method,
    headers,
    redirect: "manual",
  };
  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = request.body;
  }

  const upstream = await fetch(target, init);

  // 透传后端响应（状态码 / 响应头，含 206 与 Content-Disposition 等）
  return new NextResponse(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: upstream.headers,
  });
}

export const config = {
  matcher: "/api/:path*",
};
