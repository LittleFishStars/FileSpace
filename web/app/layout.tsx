import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import AppTheme from "./_components/app_theme";
import {AccessProvider} from "./_components/access_context";
import SyncProgress from "./_components/sync_progress";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "文件空间 | FileSpace",
  description: "局域网文件共享",
  applicationName: "文件空间",
  appleWebApp: {
    title: "文件空间",
    capable: true,
    statusBarStyle: "default",
  },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="zh"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <AppTheme>
          {/* 全局访问来源（本机/远程）上下文：顶层统一拉取一次 /api/node */}
          <AccessProvider>
            {children}
            {/* 右下角浮动同步进度条：有活跃同步任务时显示（仅本机访问时轮询状态） */}
            <SyncProgress />
          </AccessProvider>
        </AppTheme>
      </body>
    </html>
  );
}
