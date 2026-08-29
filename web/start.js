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
//      /api/* 请求由 proxy.ts 转发给后端；
//   4. 运行期间持续探测后端健康（/api/node）：一旦获取不到后端（崩溃/假死），
//      自动重启后端并更新 FILESPACE_BACKEND 环境变量（proxy.ts 每次请求读取，
//      端口变更无缝生效），前端服务自愈；重启期间 /api 请求由 proxy.ts 返回 503。
//
// 若本进程拉起了后端，退出时（Ctrl+C / kill）会一并通知后端优雅退出；
// 复用已有后端时不影响其生命周期。
//
// 本文件不经过 Next.js 编译，运行在构建产物 web/ 目录中（与 server.js 同级）。

"use strict";

const { execFile, spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const net = require("net");
const os = require("os");
const path = require("path");

// 等待后端就绪的最长时间（后端需完成锁文件写入与端口监听）。
const BACKEND_WAIT_TIMEOUT = 15000;
const POLL_INTERVAL = 200;
// 后端健康检查间隔：发现不可达后触发自动重启。
const WATCH_INTERVAL = 2000;
// 优雅终止旧后端后的等待时间，超时强制杀死。
const STOP_GRACE = 1500;

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

function execFileAsync(file, args) {
  return new Promise((resolve, reject) => {
    execFile(file, args, { timeout: 4000 }, (err, stdout) => {
      if (err) reject(err);
      else resolve(String(stdout));
    });
  });
}

// portInUse 探测端口是否已被占用（TCP 可连接）。
function portInUse(port) {
  return new Promise((resolve) => {
    const s = net.connect({ host: "127.0.0.1", port, timeout: 300 });
    s.once("connect", () => {
      s.destroy();
      resolve(true);
    });
    s.once("error", () => resolve(false));
    s.once("timeout", () => {
      s.destroy();
      resolve(false);
    });
  });
}

// waitPortFree 等待端口释放，超时返回 false。
async function waitPortFree(port, timeout) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (!(await portInUse(port))) return true;
    await sleep(200);
  }
  return false;
}

// findPidByPort 按 TCP 监听端口查找进程 PID（尽力而为）：
//  Windows 用 netstat -ano；macOS 用系统自带 lsof；Linux 解析 /proc/net/tcp 与 /proc/*/fd。
async function findPidByPort(port) {
  if (process.platform === "win32") {
    try {
      const out = await execFileAsync("netstat", ["-ano"]);
      const re = new RegExp(
        `TCP\\s+[^\\s]+:${port}\\s+[^\\s]+\\s+LISTENING\\s+(\\d+)`
      );
      const m = out.match(re);
      return m ? parseInt(m[1], 10) : 0;
    } catch {
      return 0;
    }
  }
  if (process.platform === "darwin") {
    try {
      const out = await execFileAsync("lsof", ["-ti", `tcp:${port}`, "-sTCP:LISTEN"]);
      const pid = parseInt(out.trim().split(/\s+/)[0], 10);
      return Number.isFinite(pid) ? pid : 0;
    } catch {
      return 0;
    }
  }
  // Linux：/proc/net/tcp(6) 监听行 inode → /proc/<pid>/fd 中的 socket inode
  const inodes = new Set();
  for (const file of ["/proc/net/tcp", "/proc/net/tcp6"]) {
    let text;
    try {
      text = fs.readFileSync(file, "utf8");
    } catch {
      continue;
    }
    for (const line of text.split("\n").slice(1)) {
      const parts = line.trim().split(/\s+/);
      if (parts.length < 10 || parts[3] !== "0A") continue; // 0A = LISTEN
      const lp = parts[1].split(":")[1];
      if (lp && parseInt(lp, 16) === port) inodes.add(parts[9]);
    }
  }
  for (const pid of fs.readdirSync("/proc")) {
    if (!/^\d+$/.test(pid)) continue;
    let fds;
    try {
      fds = fs.readdirSync(`/proc/${pid}/fd`);
    } catch {
      continue; // 权限不足或进程已退出
    }
    for (const fd of fds) {
      let link;
      try {
        link = fs.readlinkSync(`/proc/${pid}/fd/${fd}`);
      } catch {
        continue;
      }
      const m = link.match(/^socket:\[(\d+)\]$/);
      if (m && inodes.has(m[1])) return parseInt(pid, 10);
    }
  }
  return 0;
}

// isFilespaceProcess 判断 PID 是否属于 filespace 进程（防止误杀端口上的其他程序）。
async function isFilespaceProcess(pid) {
  if (process.platform === "win32") {
    try {
      const out = await execFileAsync("tasklist", ["/FI", `PID eq ${pid}`, "/FO", "CSV", "/NH"]);
      return /filespace/i.test(out);
    } catch {
      return false;
    }
  }
  if (process.platform === "darwin") {
    try {
      const out = await execFileAsync("ps", ["-p", String(pid), "-o", "comm="]);
      return /filespace/i.test(out);
    } catch {
      return false;
    }
  }
  try {
    const cmdline = fs.readFileSync(`/proc/${pid}/cmdline`, "utf8");
    const comm = fs.readFileSync(`/proc/${pid}/comm`, "utf8");
    return /filespace/i.test(cmdline) || /filespace/i.test(comm);
  } catch {
    return false;
  }
}

// stopChild 优雅终止本进程拉起的后端子进程，超时强杀。
async function stopChild(c) {
  if (c.exitCode !== null) return;
  if (process.platform === "win32") c.kill();
  else c.kill("SIGTERM");
  await sleep(STOP_GRACE);
  if (c.exitCode === null) c.kill("SIGKILL");
}

// stopPid 终止指定 PID 的进程（尽力而为），先 SIGTERM 后强杀。
async function stopPid(pid) {
  try {
    if (process.platform === "win32") {
      await execFileAsync("taskkill", ["/F", "/PID", String(pid)]);
      return;
    }
    process.kill(pid, "SIGTERM");
    await sleep(STOP_GRACE);
    try {
      process.kill(pid, 0); // 仍在运行 → 强杀
      process.kill(pid, "SIGKILL");
    } catch {
      // 已退出
    }
  } catch (err) {
    console.error(`终止进程 ${pid} 失败: ${err.message}`);
  }
}

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
  const started = await ensureBackend();
  // 后端端口与后端子进程句柄：运行期守护可能重启后端并更新这两个引用
  let backendPort = started.port;
  let child = started.child;

  // 前端端口：默认 8080，被占用（含后端已占用）时顺延
  const listenPort = await firstFreePort(
    parseInt(process.env.PORT || "8080", 10) || 8080
  );
  if (!listenPort) throw new Error("前端端口均被占用");

  // 交给 proxy.ts 与 server.js 的环境变量
  process.env.PORT = String(listenPort);
  process.env.FILESPACE_BACKEND = `http://127.0.0.1:${backendPort}`;

  // 运行期守护：后端不可达时自动重启（见文件头注释第 4 条）。
  // 重启后更新引用与 FILESPACE_BACKEND（proxy.ts 每次请求读取，端口变更无缝生效）。
  let restarting = false;
  async function restartBackend(reason) {
    if (restarting) return;
    restarting = true;
    console.log(`⚠️  后端不可达（${reason}），尝试自动重启...`);
    try {
      // 1) 停掉旧后端实例：本进程拉起的直接终止；复用的按端口找进程（确认是 filespace）后终止
      if (child && child.exitCode === null) {
        await stopChild(child);
      } else {
        const pid = await findPidByPort(backendPort);
        if (pid && (await isFilespaceProcess(pid))) {
          console.log(`   终止残留后端进程（PID ${pid}）`);
          await stopPid(pid);
          await waitPortFree(backendPort, STOP_GRACE + 2000);
        }
      }
      // 2) 在原端口（若已释放）或新空闲端口拉起后端
      const port = await firstFreePort(backendPort);
      if (!port) throw new Error("8080-8099 均被占用，无法重启后端");
      const exe = locateBackend();
      const next = spawn(exe, ["-p", String(port)], {
        cwd: process.cwd(), // 与直接执行 filespace 一致：默认共享当前目录
        stdio: "inherit",
      });
      next.on("error", (err) => {
        console.error(`启动后端失败（${exe}）: ${err.message}`);
      });
      // 3) 轮询等待后端就绪，成功后切换引用与环境变量
      const deadline = Date.now() + BACKEND_WAIT_TIMEOUT;
      while (Date.now() < deadline) {
        if (await backendAlive(port)) {
          child = next;
          backendPort = port;
          process.env.FILESPACE_BACKEND = `http://127.0.0.1:${port}`;
          console.log(`✅ 后端已重启并恢复（端口 ${port}）`);
          return;
        }
        if (next.exitCode !== null) break;
        await sleep(POLL_INTERVAL);
      }
      next.kill();
      throw new Error(`等待后端重启超时（端口 ${port}，程序 ${exe}）`);
    } catch (err) {
      console.error(`后端重启失败: ${err.message}`);
    } finally {
      restarting = false;
    }
  }

  // 健康检查循环：每 WATCH_INTERVAL 探测一次，发现不可达即触发重启
  setInterval(async () => {
    if (restarting) return;
    if (!(await backendAlive(backendPort))) {
      await restartBackend("健康检查无响应");
    }
  }, WATCH_INTERVAL);

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
