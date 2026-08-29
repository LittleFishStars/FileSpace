#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
文件空间 FileSpace 构建与打包脚本

构建：
    python3 scripts/build.py                   # 编译全部平台
    python3 scripts/build.py linux             # 只编译 linux
    python3 scripts/build.py windows darwin    # 编译多个指定平台
    python3 scripts/build.py --list            # 列出支持的平台
    python3 scripts/build.py --clean           # 清理构建产物

打包（自动确保先构建产物）：
    python3 scripts/build.py pack              # 打包全部可打包平台（windows + linux）
    python3 scripts/build.py pack windows      # Windows：.msi 安装包
    python3 scripts/build.py pack linux        # Linux：deb + pacman + AppImage

架构：前后端合并为一个程序（filespace 后端 Go 二进制）。
  不带参数：只启动后端 API。
  --web 参数：后端托管前端静态资源（output: 'export' → web/out/，由 go:embed 嵌入），
            在浏览器中打开界面。

产物输出到 build/ 下，后端按平台分目录：
    build/<平台>/     后端（filespace，含嵌入的前端静态资源）

构建流程：
    1. pnpm build（output: 'export' → web/out/）
    2. 将 web/out/ 拷贝到 backend/cmd/filespace/web/（go:embed 嵌入源）
    3. go build 交叉编译（各平台）

安装包输出到 build/packages/<平台>/：
    build/packages/windows/FileSpace-<版本>.msi
    build/packages/linux/filespace_<版本>_amd64.deb
    build/packages/linux/filespace-<版本>-1-x86_64.pkg.tar.zst
    build/packages/linux/FileSpace-<版本>-x86_64.AppImage

依赖的系统工具（缺失时脚本会提示安装命令）：
    wixl / wixl-heat（msitools）→ Windows .msi
    dpkg-deb（dpkg）          → Linux .deb
    makepkg（base-devel）     → Linux pacman 包
    mksquashfs（squashfs-tools）→ AppImage（type2 runtime 自动缓存）
    rsvg-convert（librsvg）   → 应用图标生成（可选）
"""

import argparse
import os
import shutil
import subprocess
import sys

# 确保 scripts/ 目录在 sys.path，支持直接运行脚本
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from _build_common import BUILD_DIR, EMBED_DIR, PLATFORMS, WEB_EXPORT
from _build_compile import build_platforms
from _build_pack import do_pack


def clean():
    """清理构建产物与安装包，保留 build/ 下的 Go 构建缓存。"""
    print("==> 清理构建产物 ...")
    for name in PLATFORMS:
        d = os.path.join(BUILD_DIR, name)
        if os.path.isdir(d):
            shutil.rmtree(d)
            print("   已删除 %s" % d)
    # 旧版布局（build/backend/<平台>/）清理
    backend_dir = os.path.join(BUILD_DIR, "backend")
    if os.path.isdir(backend_dir):
        shutil.rmtree(backend_dir)
        print("   已删除 %s" % backend_dir)
    # 旧版布局（build/web/，前后端分离时代的 standalone 前端）清理
    web_dir = os.path.join(BUILD_DIR, "web")
    if os.path.isdir(web_dir):
        shutil.rmtree(web_dir)
        print("   已删除 %s" % web_dir)
    if os.path.isdir(WEB_EXPORT):
        shutil.rmtree(WEB_EXPORT)
        print("   已删除 %s" % WEB_EXPORT)
    if os.path.isdir(EMBED_DIR):
        shutil.rmtree(EMBED_DIR)
        print("   已删除 %s" % EMBED_DIR)
        # 恢复仓库占位文件 .gitkeep（被 git 强制跟踪，删除会使工作树不干净；
        # 且 go:embed 编译期要求该目录存在，恢复后可直接 go run / make dev）
        keep = os.path.join(EMBED_DIR, ".gitkeep")
        os.makedirs(EMBED_DIR, exist_ok=True)
        open(keep, "w", encoding="utf-8").close()
    packages_dir = os.path.join(BUILD_DIR, "packages")
    if os.path.isdir(packages_dir):
        shutil.rmtree(packages_dir)
        print("   已删除 %s" % packages_dir)
    print("（保留 build/.gocache 与 build/.gomod 缓存，加快下次构建）")


def list_platforms():
    """打印支持的平台与打包格式。"""
    print("支持的平台：")
    for name, p in PLATFORMS.items():
        print("  %-14s %s" % (name, p.description))
    print("不传平台参数时编译全部平台。")
    print("\n打包格式（build.py pack [平台]）：")
    print("  windows → .msi 安装包（msitools）")
    print("  linux   → deb + pacman + AppImage")
    print("  darwin* → 暂无安装包格式，仅编译产物")


def validate_targets(targets):
    """校验平台名，存在未知平台时报错退出。"""
    unknown = [t for t in targets if t not in PLATFORMS]
    if unknown:
        sys.exit("错误：未知平台 %s，支持：%s" % (", ".join(unknown), ", ".join(PLATFORMS)))


def main():
    parser = argparse.ArgumentParser(
        description="文件空间 FileSpace 构建/打包脚本",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="例如：\n"
               "  python3 scripts/build.py                 # 编译全部平台\n"
               "  python3 scripts/build.py pack windows    # 打包 Windows 安装包\n"
               "  python3 scripts/build.py pack            # 打包 windows + linux")
    parser.add_argument(
        "platforms", nargs="*", metavar="参数",
        help="要编译的平台（可多个），或 pack [平台...] 打包安装包：%s" % " / ".join(PLATFORMS))
    parser.add_argument("--list", action="store_true", help="列出支持的平台与打包格式后退出")
    parser.add_argument("--clean", action="store_true", help="清理构建产物后退出")
    args = parser.parse_args()

    try:
        if args.list:
            list_platforms()
            return
        if args.clean:
            clean()
            return

        if args.platforms and args.platforms[0] == "pack":
            pack_targets = args.platforms[1:] or ["windows", "linux"]
            validate_targets(pack_targets)
            do_pack(pack_targets)
            return

        targets = args.platforms or list(PLATFORMS)
        validate_targets(targets)
        build_platforms(targets)
    except subprocess.CalledProcessError as e:
        sys.exit("错误：命令执行失败（退出码 %s）：%s" % (e.returncode, " ".join(e.cmd)))


if __name__ == "__main__":
    main()
