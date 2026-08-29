#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FileSpace 构建脚本 - 公共常量与工具函数

提供路径常量、平台定义、通用命令执行、版本读取等共享能力，
供 _build_compile / _build_pack / build 三个模块导入。
"""

import os
import re
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
PACKAGES_DIR = os.path.join(BUILD_DIR, "packages")  # 安装包根目录
WEB_EXPORT = os.path.join(WEB_DIR, "out")     # pnpm build（output: export）的输出目录
EMBED_DIR = os.path.join(BACKEND_DIR, "cmd", "filespace", "web")  # go:embed 嵌入源目录

APP_NAME = "文件空间 FileSpace"
APP_BINARY = "filespace"
APP_URL = "https://github.com/LittleFishStars/FileSpace"
MAINTAINER = "FileSpace Developers"

# 打包工具的缺失提示（key 为命令名）
TOOL_HINTS = {
    "wixl": "Windows .msi 需要 msitools，安装：sudo pacman -S msitools",
    "dpkg-deb": "Linux .deb 需要 dpkg，安装：sudo pacman -S dpkg",
    "makepkg": "Linux pacman 包需要 makepkg，安装：sudo pacman -S base-devel",
    "mksquashfs": "AppImage 需要 squashfs-tools，安装：sudo pacman -S squashfs-tools",
    "rsvg-convert": "图标生成需要 librsvg，安装：sudo pacman -S librsvg",
}

# AppImage type2 runtime（拼接在 AppImage 文件头部的引导程序）
APPIMAGE_RUNTIME_URL = ("https://github.com/AppImage/type2-runtime/releases/"
                        "download/continuous/runtime-x86_64")


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


def require_tool(cmd):
    """查找命令，缺失时报错退出并给出安装提示。"""
    path = shutil.which(cmd)
    if path is None:
        sys.exit("错误：缺少命令 %s。%s" % (cmd, TOOL_HINTS.get(cmd, "请先安装对应工具。")))
    return path


def read_version():
    """从 backend/version.go 读取版本号。"""
    try:
        with open(os.path.join(BACKEND_DIR, "version.go"), encoding="utf-8") as f:
            for line in f:
                m = re.search(r'Version\s*=\s*"([^"]+)"', line)
                if m:
                    return m.group(1)
    except OSError:
        pass
    return "0.0.0"


def binary_name(platform):
    """目标平台的后端可执行文件名。"""
    return APP_BINARY + ".exe" if platform.goos == "windows" else APP_BINARY


def strip_copy(src, dst):
    """拷贝二进制并去除调试符号（减小安装包体积）。"""
    shutil.copy2(src, dst)
    os.chmod(dst, 0o755)
    if shutil.which("strip"):
        try:
            run(["strip", "--strip-unneeded", dst], quiet=True)
        except subprocess.CalledProcessError:
            pass  # 无法 strip 时保留原始副本


def ensure_icon():
    """生成 256x256 应用图标 png，返回路径；无转换工具时返回 None。"""
    png = os.path.join(BUILD_DIR, "tools", "icons", "filespace.png")
    if os.path.isfile(png):
        return png
    svg = os.path.join(SCRIPTS_DIR, "assets", "filespace.svg")
    os.makedirs(os.path.dirname(png), exist_ok=True)
    rsvg = shutil.which("rsvg-convert")
    if rsvg:
        run([rsvg, "-w", "256", "-h", "256", "-o", png, svg], quiet=True)
        return png
    convert = shutil.which("convert")
    if convert:
        run(["convert", "-background", "none", "-resize", "256x256", svg, png], quiet=True)
        return png
    print("   ⚠️ 缺少 rsvg-convert / convert，跳过应用图标（%s）" % TOOL_HINTS["rsvg-convert"])
    return None


def write_desktop(path):
    """写 Linux 桌面入口文件。"""
    content = (
        "[Desktop Entry]\n"
        "Type=Application\n"
        "Name=文件空间 FileSpace\n"
        "Comment=局域网文件共享工具（P2P + mDNS）\n"
        "Exec=filespace --web\n"
        "Icon=filespace\n"
        "Terminal=true\n"
        "Categories=Network;FileTransfer;\n"
    )
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
