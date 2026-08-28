#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
文件空间 FileSpace 构建脚本

用法：
    python3 scripts/build.py                   # 编译全部平台
    python3 scripts/build.py linux             # 只编译 linux
    python3 scripts/build.py windows darwin    # 编译多个指定平台
    python3 scripts/build.py --list            # 列出支持的平台
    python3 scripts/build.py --clean           # 清理构建产物

产物输出到 build/<平台>/（每个平台目录是一个完整可分发单元）：
    build/linux/filespace         + build/linux/web/
    build/windows/filespace.exe   + build/windows/web/
    build/darwin/filespace        + build/darwin/web/
    build/darwin-amd64/filespace  + build/darwin-amd64/web/

运行示例：cd build/linux && ./filespace
"""

import argparse
import os
import shutil
import subprocess
import sys
from dataclasses import dataclass

# ---- 项目路径 ----
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEB_DIR = os.path.join(ROOT, "web")          # 前端
BACKEND_DIR = os.path.join(ROOT, "backend")  # 后端
BUILD_DIR = os.path.join(ROOT, "build")      # 产物根目录
WEB_OUT = os.path.join(WEB_DIR, "out")       # pnpm build 的静态导出产物


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


def run(cmd, cwd=None, env=None):
    """打印并执行命令，失败即抛出异常。"""
    print("==> " + " ".join(cmd))
    subprocess.run(cmd, cwd=cwd, env=env, check=True)


def build_web():
    """构建前端静态资源（平台无关，只构建一次）。"""
    print("\n[前端] 构建静态资源 ...")
    run(["pnpm", "build"], cwd=WEB_DIR)
    if not os.path.isdir(WEB_OUT):
        sys.exit("错误：前端构建未产出 %s，请检查 web/ 构建配置" % WEB_OUT)


def binary_name(platform):
    """目标平台的可执行文件名。"""
    return "filespace.exe" if platform.goos == "windows" else "filespace"


def build_backend(platform, out_dir):
    """交叉编译后端二进制到 out_dir。

    编译/模块缓存放到项目内 build/.gocache 与 build/.gomod：
    避免依赖用户全局缓存（如只读环境），也加速重复构建。
    """
    binary = os.path.join(out_dir, binary_name(platform))
    env = dict(os.environ)
    env["GOOS"] = platform.goos
    env["GOARCH"] = platform.goarch
    env["GOCACHE"] = os.path.join(BUILD_DIR, ".gocache")
    env["GOMODCACHE"] = os.path.join(BUILD_DIR, ".gomod")
    os.makedirs(env["GOCACHE"], exist_ok=True)
    os.makedirs(env["GOMODCACHE"], exist_ok=True)
    run(["go", "build", "-o", binary, "./cmd/filespace"], cwd=BACKEND_DIR, env=env)


def build_platforms(targets):
    """构建指定平台列表：前端构建一次，后端按平台逐个交叉编译。"""
    build_web()
    for name in targets:
        p = PLATFORMS[name]
        print("\n[%s] %s" % (p.name, p.description))
        plat_dir = os.path.join(BUILD_DIR, p.name)
        os.makedirs(plat_dir, exist_ok=True)

        # 1) 拷贝平台无关的前端静态资源
        web_target = os.path.join(plat_dir, "web")
        if os.path.isdir(web_target):
            shutil.rmtree(web_target)
        shutil.copytree(WEB_OUT, web_target)

        # 2) 交叉编译后端
        build_backend(p, plat_dir)

        print("   ✅ %s" % os.path.join(plat_dir, binary_name(p)))

    print("\n✅ 构建完成，产物目录：")
    for name in targets:
        print("   build/%s/  （运行：cd build/%s && ./%s）" % (
            name, name, "filespace.exe" if PLATFORMS[name].goos == "windows" else "filespace"))


def clean():
    """清理 build/<平台>/ 产物与 web/out/，保留 build/ 下的 Go 构建缓存。"""
    print("==> 清理构建产物 ...")
    for name in PLATFORMS:
        d = os.path.join(BUILD_DIR, name)
        if os.path.isdir(d):
            shutil.rmtree(d)
            print("   已删除 %s" % d)
    if os.path.isdir(WEB_OUT):
        shutil.rmtree(WEB_OUT)
        print("   已删除 %s" % WEB_OUT)
    print("（保留 build/.gocache 与 build/.gomod 缓存，加快下次构建）")


def list_platforms():
    """打印支持的平台列表。"""
    print("支持的平台：")
    for name, p in PLATFORMS.items():
        print("  %-14s %s" % (name, p.description))
    print("不传平台参数时编译全部平台。")


def main():
    usage = "例如：python3 scripts/build.py windows darwin"
    parser = argparse.ArgumentParser(
        description="文件空间 FileSpace 构建脚本：编译前端 + 后端到 build/<平台>/",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=usage)
    parser.add_argument(
        "platforms", nargs="*", metavar="平台",
        help="要编译的平台，可多个（缺省编译全部）：%s" % " / ".join(PLATFORMS))
    parser.add_argument("--list", action="store_true", help="列出支持的平台后退出")
    parser.add_argument("--clean", action="store_true", help="清理构建产物后退出")
    args = parser.parse_args()

    if args.list:
        list_platforms()
        return
    if args.clean:
        clean()
        return

    targets = args.platforms or list(PLATFORMS)
    unknown = [t for t in targets if t not in PLATFORMS]
    if unknown:
        sys.exit("错误：未知平台 %s，支持：%s" % (", ".join(unknown), ", ".join(PLATFORMS)))

    build_platforms(targets)


if __name__ == "__main__":
    main()
