#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FileSpace 构建脚本 - 编译模块

负责前端静态导出（pnpm build）、拷贝嵌入源、后端 Go 交叉编译，
以及 Windows 平台的可执行文件图标/版本信息资源嵌入（go-winres）。
"""

import os
import re
import shutil
import sys

# 确保 scripts/ 目录在 sys.path，支持直接运行脚本
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from _build_common import (
    BACKEND_DIR, BUILD_DIR, EMBED_DIR, PLATFORMS, SCRIPTS_DIR, WEB_DIR, WEB_EXPORT,
    binary_name, run,
)

# ---- Windows 资源嵌入相关常量 ----
WINRES_TOOL = os.path.join(BUILD_DIR, "tools", "bin", "go-winres")  # go-winres 工具路径
WINRES_GO_PKG = "github.com/tc-hib/go-winres@latest"                # go-winres 安装源
ICON_SVG = os.path.join(SCRIPTS_DIR, "assets", "filespace.svg")     # 图标矢量源
ICON_ICO = os.path.join(SCRIPTS_DIR, "assets", "filespace.ico")     # 生成的 Windows 图标
ICON_SIZES = [16, 24, 32, 48, 64, 128, 256]                         # ICO 包含的尺寸
SYSO_PREFIX = "filespace"                                           # syso 输出前缀（自动加平台后缀）
VERSION_FILE = os.path.join(BACKEND_DIR, "version.go")              # 版本号权威来源
PKG_DIR = os.path.join(BACKEND_DIR, "cmd", "filespace")             # 后端主包目录


def _go_env():
    """返回带项目内 Go 缓存/模块/工具路径的环境变量（避免依赖只读的全局缓存）。"""
    env = dict(os.environ)
    gocache = os.path.join(BUILD_DIR, ".gocache")
    gomod = os.path.join(BUILD_DIR, ".gomod")
    gopath = os.path.join(BUILD_DIR, ".gopath")
    gobin = os.path.join(BUILD_DIR, "tools", "bin")
    for d in (gocache, gomod, gopath, gobin):
        os.makedirs(d, exist_ok=True)
    env["GOCACHE"] = gocache
    env["GOMODCACHE"] = gomod
    env["GOPATH"] = gopath
    env["GOBIN"] = gobin
    return env


def read_version():
    """从 backend/version.go 读取版本号（如 0.3.1-2608301102）。"""
    text = open(VERSION_FILE, encoding="utf-8").read()
    m = re.search(r'Version\s*=\s*"([^"]+)"', text)
    if not m:
        sys.exit("错误：无法从 %s 读取版本号" % VERSION_FILE)
    return m.group(1)


def ensure_windows_icon():
    """确保 Windows 图标（scripts/assets/filespace.ico）存在且与 SVG 源保持同步。

    源 SVG 更新后自动重新生成多尺寸 ICO（rsvg-convert 渲染 + Pillow 打包）。
    """
    if os.path.isfile(ICON_ICO) and os.path.getmtime(ICON_ICO) >= os.path.getmtime(ICON_SVG):
        return ICON_ICO
    print("   [图标] 源 SVG 已更新，重新生成 %s ..." % ICON_ICO)
    tmp_dir = os.path.join(BUILD_DIR, "tools", "icon-work")
    os.makedirs(tmp_dir, exist_ok=True)
    try:
        for s in ICON_SIZES:
            png = os.path.join(tmp_dir, "icon_%d.png" % s)
            run(["rsvg-convert", "-w", str(s), "-h", str(s), "-o", png, ICON_SVG], quiet=True)
        try:
            from PIL import Image
        except ImportError:
            sys.exit("错误：生成 ICO 需要 Pillow（python3 -m pip install pillow）")
        base = Image.open(os.path.join(tmp_dir, "icon_%d.png" % ICON_SIZES[-1])).convert("RGBA")
        base.save(ICON_ICO, format="ICO", sizes=[(s, s) for s in ICON_SIZES])
    finally:
        shutil.rmtree(tmp_dir, ignore_errors=True)
    print("   ✅ %s" % ICON_ICO)
    return ICON_ICO


def ensure_go_winres():
    """确保 go-winres 工具可用（build/tools/bin/go-winres），缺失时安装到项目内。"""
    if os.path.isfile(WINRES_TOOL):
        return WINRES_TOOL
    print("   go-winres 未安装，正在安装（go install %s）..." % WINRES_GO_PKG)
    run(["go", "install", WINRES_GO_PKG], env=_go_env(), quiet=True)
    if not os.path.isfile(WINRES_TOOL):
        sys.exit("错误：go-winres 安装失败，请检查网络后重试")
    return WINRES_TOOL


def _prepare_windows_syso(goarch):
    """在包目录（backend/cmd/filespace/）生成 Windows 资源 syso（图标 + 版本信息）。

    返回 syso 路径；go build 完成后由调用方删除。
    文件名带 _windows_<arch> 平台后缀，Go 工具链只在对应平台构建时自动链接。
    """
    tool = ensure_go_winres()
    icon = ensure_windows_icon()
    version = read_version().replace("-", ".")  # 0.3.1-2608301102 → 0.3.1.2608301102（Windows 四段）
    syso = os.path.join(PKG_DIR, "%s_windows_%s.syso" % (SYSO_PREFIX, goarch))
    run([
        tool, "simply",
        "--icon", icon,
        "--out", SYSO_PREFIX,
        "--arch", goarch,
        "--product-name", "文件空间 FileSpace",
        "--file-description", "FileSpace 局域网文件共享工具",
        "--product-version", version,
        "--file-version", version,
        "--original-filename", "filespace.exe",
        "--copyright", "(c) 2026 FileSpace",
    ], cwd=PKG_DIR, quiet=True)
    if not os.path.isfile(syso):
        sys.exit("错误：Windows 资源生成失败（%s 未生成）" % syso)
    return syso


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
    env = _go_env()
    env["GOOS"] = platform.goos
    env["GOARCH"] = platform.goarch
    run(["go", "build", "-o", binary, pkg], cwd=BACKEND_DIR, env=env)


def build_backend(platform):
    """交叉编译后端程序（filespace：API + 嵌入的前端静态资源）到 build/<平台>/。

    Windows 平台额外嵌入图标与版本信息资源（syso），构建后自动清理。
    """
    out_dir = os.path.join(BUILD_DIR, platform.name)
    os.makedirs(out_dir, exist_ok=True)
    syso = None
    if platform.goos == "windows":
        # 在包目录生成 Windows 资源（图标 + 版本信息），go build 会自动链接
        syso = _prepare_windows_syso(platform.goarch)
    try:
        go_build(platform, out_dir, os.path.join(out_dir, binary_name(platform)), "./cmd/filespace")
    finally:
        if syso and os.path.isfile(syso):
            os.remove(syso)
            print("   已清理构建期 Windows 资源 %s" % os.path.basename(syso))


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
