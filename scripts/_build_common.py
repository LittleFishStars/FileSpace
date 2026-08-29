#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FileSpace 构建脚本 - 公共常量与工具函数

提供路径常量、平台定义、通用命令执行等共享能力，
供 _build_compile / build 两个模块导入。
"""

import os
import shutil
import subprocess
import sys
from dataclasses import dataclass

# ---- 项目路径 ----
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPTS_DIR = os.path.join(ROOT, "scripts")   # 脚本目录
WEB_DIR = os.path.join(ROOT, "web")           # 前端
BACKEND_DIR = os.path.join(ROOT, "backend")   # 后端
BUILD_DIR = os.path.join(ROOT, "build")       # 产物根目录
WEB_EXPORT = os.path.join(WEB_DIR, "out")     # pnpm build（output: export）的输出目录
EMBED_DIR = os.path.join(BACKEND_DIR, "cmd", "filespace", "web")  # go:embed 嵌入源目录

APP_BINARY = "filespace"


@dataclass(frozen=True)
class Platform:
    """一个目标平台的交叉编译参数。"""

    name: str         # 命令行参数名（同时作为产物目录名）
    goos: str         # GOOS
    goarch: str       # GOARCH
    description: str  # 平台说明（提示用）


PLATFORMS = {
    "linux": Platform("linux", "linux", "amd64", "Linux x86_64"),
    "windows": Platform("windows", "windows", "amd64", "Windows x86_64"),
    "darwin": Platform("darwin", "darwin", "arm64", "macOS Apple Silicon"),
    "darwin-amd64": Platform("darwin-amd64", "darwin", "amd64", "macOS Intel"),
}


def run(cmd, cwd=None, env=None, quiet=False):
    """执行命令，失败即抛出异常；quiet=True 时不回显命令行（辅助命令）。"""
    if not quiet:
        print("==> " + " ".join(cmd))
    subprocess.run(cmd, cwd=cwd, env=env, check=True)


def binary_name(platform):
    """目标平台的后端可执行文件名。"""
    return APP_BINARY + ".exe" if platform.goos == "windows" else APP_BINARY
