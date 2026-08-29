#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
文件空间 FileSpace 开发启动脚本（前端自动拉起后端）

前端（Next.js dev server）启动时读取后端锁文件获取端口；若后端尚未启动，
则自动拉起一个后端（go run，端口取空闲端口），并把实际端口通过
FILESPACE_BACKEND 环境变量交给 next dev（web/proxy.ts 据此反代 /api/*）。

运行期间守护线程持续探测后端健康：一旦获取不到后端（崩溃/假死），自动重启
后端。注意：next dev 子进程的环境变量无法在线更新，因此重启必须保持原端口
（会先终止占用该端口的残留 filespace 进程）；若端口被其他程序占用无法释放，
则无法自愈，需重启 make dev。

用法：
    python3 scripts/dev.py        # 等价 make dev-web / make dev
    make dev-backend              # 只启动后端（不自动拉起）
"""

import json
import os
import platform
import re
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB_DIR = ROOT / "web"
BACKEND_DIR = ROOT / "backend"

# go run 首次需要编译，等待后端就绪的超时放宽
READY_TIMEOUT = 60
POLL_INTERVAL = 0.3
# 守护线程健康检查间隔 / 优雅终止旧后端后的等待时间
WATCH_INTERVAL = 2.0
STOP_GRACE = 1.5


def lock_file() -> Path:
    """后端运行锁文件：内容为运行中后端的端口。"""
    sysname = platform.system()
    if sysname == "Windows":
        base = Path(os.environ.get("APPDATA", Path.home() / "AppData" / "Roaming"))
    elif sysname == "Darwin":
        base = Path.home() / "Library" / "Application Support"
    else:
        base = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))
    return base / "filespace" / "lock"


def read_lock_port() -> int:
    """读取锁文件中的端口；文件缺失或内容无效返回 0。"""
    try:
        return int(lock_file().read_text(encoding="utf-8").strip())
    except (OSError, ValueError):
        return 0


def backend_alive(port: int) -> bool:
    """探测端口上是否运行着活的 filespace 后端。"""
    try:
        with urllib.request.urlopen(
            f"http://127.0.0.1:{port}/api/node", timeout=0.8
        ) as resp:
            return resp.status == 200 and bool(json.loads(resp.read()).get("id"))
    except Exception:
        return False


def port_in_use(port: int) -> bool:
    """端口是否已被占用。"""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def free_port(start: int = 8080) -> int:
    """从 start 起找第一个空闲端口。"""
    for port in range(start, start + 20):
        if not port_in_use(port):
            return port
    sys.exit(f"错误：端口 {start}-{start + 19} 均被占用，无法启动后端")


def _start_backend(port: int) -> subprocess.Popen:
    """在指定端口拉起后端（go run，独立会话进程组）。"""
    print(f"==> 拉起后端（go run ./cmd/filespace -p {port}）...")
    return subprocess.Popen(
        ["go", "run", "./cmd/filespace", "-p", str(port)],
        cwd=BACKEND_DIR,
        start_new_session=True,
    )


def find_pid_by_port(port: int) -> int:
    """按 TCP 监听端口查找进程 PID（尽力而为），找不到返回 0。

    Windows 用 netstat -ano；macOS 用系统自带 lsof；
    Linux 解析 /proc/net/tcp(6) 监听行 inode 与 /proc/*/fd 的 socket inode。
    """
    sysname = platform.system()
    if sysname == "Windows":
        try:
            out = subprocess.run(
                ["netstat", "-ano"], capture_output=True, text=True, timeout=5
            ).stdout
            for line in out.splitlines():
                parts = line.split()
                if len(parts) >= 5 and parts[0] == "TCP" and parts[3] == "LISTENING":
                    if parts[1].rsplit(":", 1)[-1] == str(port):
                        return int(parts[4])
        except (OSError, ValueError, subprocess.SubprocessError):
            pass
        return 0
    if sysname == "Darwin":
        try:
            out = subprocess.run(
                ["lsof", "-ti", f"tcp:{port}", "-sTCP:LISTEN"],
                capture_output=True, text=True, timeout=5,
            )
            if out.returncode == 0 and out.stdout.strip():
                return int(out.stdout.split()[0])
        except (OSError, ValueError, subprocess.SubprocessError):
            pass
        return 0
    # Linux
    hexport = f"{port:04X}"
    inodes = set()
    for path in ("/proc/net/tcp", "/proc/net/tcp6"):
        try:
            with open(path, encoding="utf-8") as f:
                text = f.read()
        except OSError:
            continue
        for line in text.splitlines()[1:]:
            parts = line.split()
            # 监听状态 0A，本地地址 <ip>:<16 进制端口>
            if len(parts) >= 10 and parts[3] == "0A" and parts[1].endswith(f":{hexport}"):
                inodes.add(parts[9])
    if not inodes:
        return 0
    try:
        pids = os.listdir("/proc")
    except OSError:
        return 0
    for pid in pids:
        if not pid.isdigit():
            continue
        try:
            fd_dir = f"/proc/{pid}/fd"
            for fd in os.listdir(fd_dir):
                link = os.readlink(os.path.join(fd_dir, fd))
                m = re.match(r"^socket:\[(\d+)\]$", link)
                if m and m.group(1) in inodes:
                    return int(pid)
        except OSError:
            continue
    return 0


def is_filespace_pid(pid: int) -> bool:
    """判断 PID 是否属于 filespace 进程（防止误杀端口上的其他程序）。"""
    sysname = platform.system()
    if sysname == "Windows":
        try:
            out = subprocess.run(
                ["tasklist", "/FI", f"PID eq {pid}", "/FO", "CSV", "/NH"],
                capture_output=True, text=True, timeout=5,
            ).stdout
            return "filespace" in out.lower()
        except (OSError, subprocess.SubprocessError):
            return False
    if sysname == "Darwin":
        try:
            out = subprocess.run(
                ["ps", "-p", str(pid), "-o", "comm="],
                capture_output=True, text=True, timeout=5,
            ).stdout
            return "filespace" in out.lower()
        except (OSError, subprocess.SubprocessError):
            return False
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as f:
            cmdline = f.read().decode(errors="ignore")
        with open(f"/proc/{pid}/comm", encoding="utf-8") as f:
            comm = f.read()
        return "filespace" in cmdline or "filespace" in comm
    except OSError:
        return False


def _kill_group(proc: subprocess.Popen) -> None:
    """终止整个进程组（go run 及其编译产物的后端进程），先优雅后强杀。"""
    try:
        os.killpg(proc.pid, signal.SIGTERM)
        proc.wait(timeout=STOP_GRACE)
    except (ProcessLookupError, subprocess.TimeoutExpired):
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def _kill_pid(pid: int) -> None:
    """终止指定 PID（尽力而为），先 SIGTERM 后强杀。"""
    if platform.system() == "Windows":
        subprocess.run(
            ["taskkill", "/F", "/PID", str(pid)], capture_output=True, timeout=5
        )
        return
    try:
        os.kill(pid, signal.SIGTERM)
        time.sleep(STOP_GRACE)
        try:
            os.kill(pid, 0)  # 仍在运行 → 强杀
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    except (ProcessLookupError, PermissionError):
        pass


def _wait_port_free(port: int, timeout: float) -> bool:
    """等待端口释放，超时返回 False。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        if not port_in_use(port):
            return True
        time.sleep(0.2)
    return False


def backend_watchdog(
    port_box: list[int],
    proc_box: list[subprocess.Popen | None],
    stop: threading.Event,
) -> None:
    """守护线程：探测后端健康，不可达时自动重启（保持原端口）。

    next dev 子进程的 FILESPACE_BACKEND 无法在线更新，所以重启必须保持原端口：
    先终止占用该端口的残留 filespace 进程，再在原端口拉起新后端。
    """
    while not stop.wait(WATCH_INTERVAL):
        port = port_box[0]
        if backend_alive(port):
            continue
        print(f"==> 后端不可达，尝试自动重启（端口 {port}）...")
        # 1) 停掉旧后端：本进程拉起的杀进程组；复用的按端口找进程（确认是 filespace）后终止
        if proc_box[0] is not None and proc_box[0].poll() is None:
            _kill_group(proc_box[0])
            proc_box[0] = None
        else:
            pid = find_pid_by_port(port)
            if pid and is_filespace_pid(pid):
                print(f"==> 终止残留后端进程（PID {pid}）...")
                _kill_pid(pid)
                _wait_port_free(port, STOP_GRACE + 2.0)
        # 2) 原端口拉起新后端并等待就绪
        proc = _start_backend(port)
        deadline = time.time() + READY_TIMEOUT
        ok = False
        while time.time() < deadline:
            if backend_alive(port):
                ok = True
                break
            if proc.poll() is not None:
                break
            time.sleep(POLL_INTERVAL)
        if ok:
            proc_box[0] = proc
            print(f"==> 后端已重启并恢复（端口 {port}）")
        else:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            print(f"错误：后端重启失败（端口 {port}），请检查后端日志")


def ensure_backend() -> tuple[int, subprocess.Popen | None]:
    """确保后端运行：读锁文件复用或自动拉起，返回 (后端端口, 子进程或 None)。"""
    p = read_lock_port()
    if p and backend_alive(p):
        print(f"==> 复用已运行的后端（端口 {p}，见锁文件 {lock_file()}）")
        return p, None

    port = free_port()
    print(f"==> 后端未启动，自动拉起（go run ./cmd/filespace -p {port}）...")
    # 独立会话（进程组）：go run 编译产物是其后端子进程，
    # 退出时对整个进程组终止，避免 go run 退出后留下孤儿后端
    proc = _start_backend(port)
    deadline = time.time() + READY_TIMEOUT
    while time.time() < deadline:
        if backend_alive(port):
            print(f"==> 后端已就绪（端口 {port}）")
            return port, proc
        if proc.poll() is not None:
            break
        time.sleep(POLL_INTERVAL)
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    sys.exit("错误：后端启动失败，请检查 Go 编译或后端日志")


def main() -> None:
    port, proc = ensure_backend()
    # 可变容器：守护线程重启后端后更新端口/进程引用，主流程（含清理）读同一份
    port_box: list[int] = [port]
    proc_box: list[subprocess.Popen | None] = [proc]
    stop = threading.Event()
    watchdog = threading.Thread(
        target=backend_watchdog, args=(port_box, proc_box, stop), daemon=True
    )
    watchdog.start()
    env = dict(os.environ)
    env["FILESPACE_BACKEND"] = f"http://127.0.0.1:{port}"
    try:
        print(f"==> 启动前端（pnpm dev，:3000，/api/* 代理到 {env['FILESPACE_BACKEND']}）...")
        web = subprocess.run(["pnpm", "dev"], cwd=WEB_DIR, env=env)
        sys.exit(web.returncode)
    finally:
        stop.set()
        if proc_box[0] is not None and proc_box[0].poll() is None:
            print("==> 停止自动拉起的后端...")
            # 终止整个进程组（go run 及其编译产物的后端进程）
            _kill_group(proc_box[0])


if __name__ == "__main__":
    main()
