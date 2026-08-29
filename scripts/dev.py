#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
文件空间 FileSpace 开发启动脚本

同时启动后端（Go API，:8080）与前端（Next.js dev server，:3000）。
前端通过 next.config.ts 的 rewrites 将 /api 反代到后端。

用法：
    python3 scripts/dev.py        # 等价 make dev / make dev-web
    make dev-backend              # 只启动后端（不自动拉起）
"""

import os
import platform
import signal
import socket
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB_DIR = ROOT / "web"
BACKEND_DIR = ROOT / "backend"

BACKEND_PORT = 8080
FRONTEND_PORT = 3000
READY_TIMEOUT = 60
POLL_INTERVAL = 0.3


def port_in_use(port: int) -> bool:
    """端口是否已被占用。"""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def wait_port(port: int, timeout: float) -> bool:
    """等待端口就绪，超时返回 False。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        if port_in_use(port):
            return True
        time.sleep(POLL_INTERVAL)
    return False


def main() -> None:
    # 检查后端端口是否被占用
    if port_in_use(BACKEND_PORT):
        print(f"==> 后端端口 {BACKEND_PORT} 已被占用，仅启动前端...")
        # 只启动前端
        try:
            web = subprocess.run(["pnpm", "dev"], cwd=WEB_DIR)
            sys.exit(web.returncode)
        except KeyboardInterrupt:
            sys.exit(0)

    # 启动后端
    print(f"==> 启动后端（go run ./cmd/filespace -p {BACKEND_PORT}）...")
    backend = subprocess.Popen(
        ["go", "run", "./cmd/filespace", "-p", str(BACKEND_PORT)],
        cwd=BACKEND_DIR,
        start_new_session=True,
    )

    try:
        # 等待后端就绪
        if not wait_port(BACKEND_PORT, READY_TIMEOUT):
            print("错误：后端启动超时", file=sys.stderr)
            sys.exit(1)

        print(f"==> 后端已就绪（端口 {BACKEND_PORT}）")
        print(f"==> 启动前端（pnpm dev，:{FRONTEND_PORT}，/api/* 反代到后端）...")

        # 启动前端
        web = subprocess.run(["pnpm", "dev"], cwd=WEB_DIR)
        sys.exit(web.returncode)
    finally:
        # 终止后端
        print("==> 停止后端...")
        if platform.system() == "Windows":
            backend.kill()
        else:
            os.killpg(backend.pid, signal.SIGTERM)
            try:
                backend.wait(timeout=3)
            except subprocess.TimeoutExpired:
                os.killpg(backend.pid, signal.SIGKILL)


if __name__ == "__main__":
    main()
