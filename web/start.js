#!/usr/bin/env node
// 文件空间 FileSpace 前端启动器（Next.js standalone 服务器）
//
// 在任意文件夹执行 `node web/start.js` 即可共享该文件夹：
//   1. 读取后端锁文件（用户配置目录 filespace/lock）获取后端端口；
//   2. 锁文件缺失、内容无效或端口无响应（崩溃残留）→ 自动拉起一个后端
//      （filespace -p <端口>，工作目录为当前目录，与直接执行 filespace 行为一致），
//      轮询等待其就绪；
//   3. 选择前端端口（默认 8080，被占用时自动顺延），设置 PORT / FILESPACE_BACKEND
//      环境变量后启动同目录下的 Next.js standalone 服务器（server.js）；
//      /api/* 请求由 proxy.ts 转发给后端。
//
// 若本进程拉起了后端，退出时（Ctrl+C / kill）会一并通知后端优雅退出；
// 复用已有后端时不影响其生命周期。
//
// 本文件不经过 Next.js 编译，运行在构建产物 web/ 目录中（与 server.js 同级）。

"use strict";

const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const net = require("net");
const os = require("os");
const path = require("path");

// 等待后端就绪的最长时间（后端需完成锁文件写入与端口监听）。
const BACKEND_WAIT_TIMEOUT = 15000;
const POLL_INTERVAL = 200;

// ---- 锁文件与后端探测 ----

// lockFile 返回后端运行锁文件路径（内容为运行中后端的端口）。
function lockFile() {
  let base;
  if (process.platform === "win32") {
    base = process.env.APPDATA || path.join(os.homedir(), "AppData", "Roaming");
  } else if (process.platform === "darwin") {
    base = path.join(os.homedir(), "Library", "Application Support");
  } else {
    base = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config");
  }
  return path.join(base, "filespace", "lock");
}

// readLockPort 读取锁文件中的端口；文件缺失或内容无效返回 0。
function readLockPort() {
  try {
    const port = parseInt(fs.readFileSync(lockFile(), "utf8").trim(), 10);
    return Number.isFinite(port) && port > 0 ? port : 0;
  } catch {
    return 0;
  }
}

// backendAlive 探测端口上是否运行着活的 filespace 后端。
function backendAlive(port) {
  return new Promise((resolve) => {
    const req = http.get(
      { host: "127.0.0.1", port, path: "/api/node", timeout: 800 },
      (res) => {
        let body = "";
        res.on("data", (c) => (body += c));
        res.on("end", () => {
          try {
            resolve(res.statusCode === 200 && JSON.parse(body).id ? true : false);
          } catch {
            resolve(false);
          }
        });
      }
    );
    req.on("error", () => resolve(false));
    req.on("timeout", () => {
      req.destroy();
      resolve(false);
    });
  });
}

// ---- 端口与后端定位 ----

// firstFreePort 从 start 起找第一个可监听的端口（探测后立即释放，存在微小竞态）。
async function firstFreePort(start) {
  for (let p = start; p < start + 20; p++) {
    const free = await new Promise((resolve) => {
      const srv = net.createServer();
      srv.once("error", () => resolve(false));
      srv.listen(p, "0.0.0.0", () => srv.close(() => resolve(true)));
    });
    if (free) return p;
  }
  return 0;
}

// locateBackend 定位后端二进制，按优先级尝试：
//  1. web/ 同级上一级的 filespace（build/filespace，单目录分发的兼容路径）
//  2. build/backend/<平台>/filespace（前后端分开、后端按平台分目录的新布局）
//  3. PATH 中的 filespace
function locateBackend() {
  const name = process.platform === "win32" ? "filespace.exe" : "filespace";
  let platformDir;
  if (process.platform === "win32") platformDir = "windows";
  else if (process.platform === "darwin")
    platformDir = process.arch === "arm64" ? "darwin" : "darwin-amd64";
  else platformDir = "linux";

  const candidates = [
    path.join(__dirname, "..", name),
    path.join(__dirname, "..", "backend", platformDir, name),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return name; // 依赖 PATH 查找
}

// ---- 后端生命周期 ----

// ensureBackend 返回后端端口与（若由本进程拉起的）后端子进程。
async function ensureBackend() {
  // 1) 锁文件优先：存在且端口存活 → 复用
  const locked = readLockPort();
  if (locked && (await backendAlive(locked))) {
    return { port: locked, child: null };
  }

  // 2) 后端未运行：找一个空闲端口，拉起后端
  const port = await firstFreePort(8080);
  if (!port) throw new Error("8080-8099 均被占用，无法启动后端");
  const exe = locateBackend();
  const child = spawn(exe, ["-p", String(port)], {
    cwd: process.cwd(), // 与直接执行 filespace 一致：默认共享当前目录
    stdio: "inherit", // 后端日志输出到当前终端
  });
  child.on("error", (err) => {
    console.error(`启动后端失败（${exe}）: ${err.message}`);
    process.exit(1);
  });

  // 3) 轮询等待后端就绪
  const deadline = Date.now() + BACKEND_WAIT_TIMEOUT;
  while (Date.now() < deadline) {
    if (await backendAlive(port)) return { port, child };
    // 竞态兜底：若其他进程（如另一个前端）先拿到了锁，复用其端口
    const other = readLockPort();
    if (other && other !== port && (await backendAlive(other))) {
      return { port: other, child };
    }
    await new Promise((r) => setTimeout(r, POLL_INTERVAL));
  }
  child.kill();
  throw new Error(`等待后端启动超时（端口 ${port}，程序 ${exe}）`);
}

// ---- 主流程 ----

async function main() {
  const { port: backendPort, child } = await ensureBackend();

  // 前端端口：默认 8080，被占用（含后端已占用）时顺延
  const listenPort = await firstFreePort(
    parseInt(process.env.PORT || "8080", 10) || 8080
  );
  if (!listenPort) throw new Error("前端端口均被占用");

  // 交给 proxy.ts 与 server.js 的环境变量
  process.env.PORT = String(listenPort);
  process.env.FILESPACE_BACKEND = `http://127.0.0.1:${backendPort}`;

  // 退出时通知本进程拉起的后端优雅退出
  const cleanup = () => {
    if (child && child.exitCode === null) {
      if (process.platform === "win32") child.kill();
      else child.kill("SIGTERM");
    }
  };
  process.on("SIGINT", cleanup);
  process.on("SIGTERM", cleanup);

  console.log(
    `后端端口: ${backendPort}${
      child ? "（本进程自动拉起）" : "（复用已有实例，见锁文件）"
    }`
  );
  console.log(`🌐 文件空间界面已启动: http://localhost:${listenPort}`);

  // 启动 Next.js standalone 服务器（读取 PORT / HOSTNAME / FILESPACE_BACKEND）
  require("./server.js");
}

main().catch((err) => {
  console.error(`前端启动失败: ${err.message}`);
  process.exit(1);
});
