#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
文件空间 FileSpace 开发启动脚本（前端自动拉起后端）

前端（Next.js dev server）启动时读取后端锁文件获取端口；若后端尚未启动，
则自动拉起一个后端（go run，端口取空闲端口），并把实际端口通过
FILESPACE_BACKEND 环境变量交给 next dev（web/proxy.ts 据此反代 /api/*）。

用法：
    python3 scripts/dev.py        # 等价 make dev-web / make dev
    make dev-backend              # 只启动后端（不自动拉起）
"""

import json
import os
import platform
import signal
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB_DIR = ROOT / "web"
BACKEND_DIR = ROOT / "backend"

# go run 首次需要编译，等待后端就绪的超时放宽
READY_TIMEOUT = 60
POLL_INTERVAL = 0.3


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
    proc = subprocess.Popen(
        ["go", "run", "./cmd/filespace", "-p", str(port)],
        cwd=BACKEND_DIR,
        start_new_session=True,
    )
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
    env = dict(os.environ)
    env["FILESPACE_BACKEND"] = f"http://127.0.0.1:{port}"
    try:
        print(f"==> 启动前端（pnpm dev，:3000，/api/* 代理到 {env['FILESPACE_BACKEND']}）...")
        web = subprocess.run(["pnpm", "dev"], cwd=WEB_DIR, env=env)
        sys.exit(web.returncode)
    finally:
        if proc is not None and proc.poll() is None:
            print("==> 停止自动拉起的后端...")
            # 终止整个进程组（go run 及其编译产物的后端进程）
            try:
                os.killpg(proc.pid, signal.SIGTERM)
                proc.wait(timeout=5)
            except (ProcessLookupError, subprocess.TimeoutExpired):
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass


if __name__ == "__main__":
    main()
