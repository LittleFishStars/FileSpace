#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FileSpace 构建脚本 - 编译模块

负责前端静态导出（pnpm build）、拷贝嵌入源、后端 Go 交叉编译。
"""

import os
import shutil
import sys

# 确保 scripts/ 目录在 sys.path，支持直接运行脚本
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from _build_common import (
    BACKEND_DIR, BUILD_DIR, EMBED_DIR, PLATFORMS, WEB_DIR, WEB_EXPORT,
    binary_name, run,
)


def build_web():
    """构建前端静态导出（output: 'export' → web/out/）。"""
    print("\n[前端] 构建静态导出 ...")
    env = dict(os.environ)
    # 版本号含超 int32 的时间戳补丁（如 0.3.0-2608292038），pnpm 11 的依赖状态检查
    # （verify-deps-before-run，默认 install）会解析出错并误判依赖过期、自动触发 pnpm install，
    # 在离线/只读环境下直接失败；这里关闭自动依赖检查（依赖变更时手动 pnpm install）。
    env["pnpm_config_verify_deps_before_run"] = "false"
    run(["pnpm", "build"], cwd=WEB_DIR, env=env)
    if not os.path.isdir(WEB_EXPORT):
        sys.exit("错误：前端构建未产出 %s，请检查 web/ 构建配置" % WEB_EXPORT)


def prepare_embed_dir():
    """将前端静态导出拷贝到 go:embed 嵌入源目录（backend/cmd/filespace/web/）。"""
    print("\n[前端] 拷贝静态资源到嵌入源目录 ...")
    if os.path.isdir(EMBED_DIR):
        shutil.rmtree(EMBED_DIR)
    shutil.copytree(WEB_EXPORT, EMBED_DIR)
    _restore_embed_placeholder()
    print("   ✅ %s" % EMBED_DIR)


def _restore_embed_placeholder():
    """恢复仓库占位文件 .gitkeep：
    go:embed 编译期要求目录存在且非空，且该文件被 git 强制跟踪，
    目录重建后需补回（空文件，对嵌入内容无影响），避免工作树显示 deleted。
    """
    keep = os.path.join(EMBED_DIR, ".gitkeep")
    if not os.path.isfile(keep):
        open(keep, "w", encoding="utf-8").close()


def go_build(platform, out_dir, binary, pkg):
    """按平台交叉编译指定 Go 包到 out_dir。

    编译/模块缓存放到项目内 build/.gocache 与 build/.gomod：
    避免依赖用户全局缓存（如只读环境），也加速重复构建。
    """
    env = dict(os.environ)
    env["GOOS"] = platform.goos
    env["GOARCH"] = platform.goarch
    env["GOCACHE"] = os.path.join(BUILD_DIR, ".gocache")
    env["GOMODCACHE"] = os.path.join(BUILD_DIR, ".gomod")
    os.makedirs(env["GOCACHE"], exist_ok=True)
    os.makedirs(env["GOMODCACHE"], exist_ok=True)
    run(["go", "build", "-o", binary, pkg], cwd=BACKEND_DIR, env=env)


def build_backend(platform):
    """交叉编译后端程序（filespace：API + 嵌入的前端静态资源）到 build/<平台>/。"""
    out_dir = os.path.join(BUILD_DIR, platform.name)
    os.makedirs(out_dir, exist_ok=True)
    go_build(platform, out_dir, os.path.join(out_dir, binary_name(platform)), "./cmd/filespace")


def build_platforms(targets):
    """构建指定平台列表：前端静态导出 → 拷贝嵌入源 → 后端按平台交叉编译。"""
    build_web()
    prepare_embed_dir()
    for name in targets:
        p = PLATFORMS[name]
        print("\n[%s] %s" % (p.name, p.description))
        build_backend(p)
        print("   ✅ %s" % os.path.join(BUILD_DIR, p.name, binary_name(p)))

    print("\n✅ 构建完成，产物目录：")
    for name in targets:
        p = PLATFORMS[name]
        print("   build/%s/  （filespace 后端，含嵌入的前端静态资源）" % name)
    print("\n运行：")
    for name in targets:
        p = PLATFORMS[name]
        exe = os.path.join(BUILD_DIR, p.name, binary_name(p))
        print("   %s            # 只启动后端 API" % exe)
        print("   %s --web      # 启动后端 + 前端界面，并在浏览器中打开" % exe)


def ensure_build(platform):
    """确保某平台构建产物存在（存在则复用，缺失则先构建）。"""
    platform_dir = os.path.join(BUILD_DIR, platform.name)
    binary = os.path.join(platform_dir, binary_name(platform))
    if os.path.isfile(binary):
        print("   复用已有构建产物：build/%s/" % platform.name)
        return
    print("   构建产物缺失，先构建 %s ..." % platform.name)
    if not os.path.isdir(WEB_EXPORT):
        build_web()
    if not os.path.isdir(EMBED_DIR):
        prepare_embed_dir()
    build_backend(platform)
