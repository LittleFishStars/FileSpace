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

产物输出到 build/ 下，前后端分开、后端按平台分目录：
    build/web/                前端（Next.js standalone，平台无关，仅需 Node.js，只构建一次）
    build/backend/<平台>/     后端（filespace，按平台交叉编译）

两个独立程序（前端即 Next.js 本身，无额外启动器程序）：
    filespace      后端：P2P + mDNS + 文件共享 API（纯 API，不托管界面）
    web/           前端：Next.js standalone 服务器（server.js + node_modules +
                   .next/ + public/ + start.js）；node web/start.js 启动时读取后端
                   锁文件获取端口，后端未启动则自动拉起一个，并把 /api 反代给后端
                   （运行：cd build && node web/start.js）

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
import glob
import os
import re
import shutil
import subprocess
import sys
import uuid
from dataclasses import dataclass

# ---- 项目路径 ----
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPTS_DIR = os.path.join(ROOT, "scripts")   # 脚本目录
WEB_DIR = os.path.join(ROOT, "web")           # 前端
BACKEND_DIR = os.path.join(ROOT, "backend")   # 后端
BUILD_DIR = os.path.join(ROOT, "build")       # 产物根目录
PACKAGES_DIR = os.path.join(BUILD_DIR, "packages")  # 安装包根目录
WEB_STANDALONE = os.path.join(WEB_DIR, ".next", "standalone")  # pnpm build 的 standalone 输出

APP_NAME = "文件空间 FileSpace"
APP_BINARY = "filespace"
APP_URL = "https://github.com/LittleFishStars/FileSpace"
MAINTAINER = "FileSpace Developers"

# 打包工具的缺失提示（key 为命令名）
TOOL_HINTS = {
    "wixl": "Windows .msi 需要 msitools，安装：sudo pacman -S msitools",
    "wixl-heat": "Windows .msi 需要 msitools，安装：sudo pacman -S msitools",
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


# ============================================================
# 构建
# ============================================================

def build_web():
    """构建前端 standalone 服务器（平台无关，只构建一次）。"""
    print("\n[前端] 构建 standalone 服务器 ...")
    run(["pnpm", "build"], cwd=WEB_DIR)
    if not os.path.isdir(WEB_STANDALONE):
        sys.exit("错误：前端构建未产出 %s，请检查 web/ 构建配置" % WEB_STANDALONE)


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
    """交叉编译后端程序（filespace：纯 API，P2P + mDNS）到 build/backend/<平台>/。"""
    out_dir = os.path.join(BUILD_DIR, "backend", platform.name)
    os.makedirs(out_dir, exist_ok=True)
    go_build(platform, out_dir, os.path.join(out_dir, binary_name(platform)), "./cmd/filespace")


def patch_standalone_deps(web_target):
    """补齐 pnpm 布局下 standalone trace 遗漏的 next 运行时依赖。

    Next standalone 的 trace 基于 npm/yarn 扁平 node_modules；pnpm 使用符号链接
    （node_modules/.pnpm/next@.../node_modules/ 内含 next 的全部可解析依赖），
    导致 trace 只输出 next/react/react-dom 三个顶层包，运行时缺 @next/*、
    @swc/helpers、styled-jsx 等模块。这里把 next 依赖上下文中的其余包
    完整（跟随符号链接）补拷到 standalone/node_modules 顶层，与官方布局一致。
    """
    nm = os.path.join(web_target, "node_modules")
    ctxs = glob.glob(os.path.join(WEB_DIR, "node_modules", ".pnpm", "next@*", "node_modules"))
    if not ctxs:
        return  # 非 pnpm 布局（npm / yarn）无需修补
    ctx = ctxs[0]
    for entry in sorted(os.listdir(ctx)):
        if entry == "next":
            continue  # next 自身已由 trace 输出
        dst = os.path.join(nm, entry)
        if os.path.lexists(dst):
            continue
        real = os.path.realpath(os.path.join(ctx, entry))
        if os.path.isdir(real):
            shutil.copytree(real, dst, symlinks=False)  # 跟随符号链接复制真实内容
        else:
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy2(real, dst)


def strip_native_deps(web_target):
    """删除平台特定 native 依赖，使前端分发平台无关（跨平台共用一份 web/）。

    经实测：Next 16 standalone 运行时为纯 JS（Turbopack 产物已编译、
    images.unoptimized 时不再加载 sharp），@next/swc-* 与 sharp/@img/* 均可安全移除，
    产物在任何平台仅需 Node.js 即可运行。
    """
    nm = os.path.join(web_target, "node_modules")
    # @next/swc-*（各平台 native 绑定，如 swc-linux-x64-gnu）
    next_dir = os.path.join(nm, "@next")
    if os.path.isdir(next_dir):
        for name in list(os.listdir(next_dir)):
            if name.startswith("swc"):
                shutil.rmtree(os.path.join(next_dir, name))
    # sharp 及其依赖树（顶层 + pnpm store 中的 @img/* 等）
    for rel in ("sharp", "detect-libc"):
        p = os.path.join(nm, rel)
        if os.path.isdir(p):
            shutil.rmtree(p)
    pnpm = os.path.join(nm, ".pnpm")
    if os.path.isdir(pnpm):
        for entry in list(os.listdir(pnpm)):
            if entry.startswith("@img+") or entry.startswith("sharp@") or entry.startswith("detect-libc@"):
                shutil.rmtree(os.path.join(pnpm, entry))
        pnpm_nm = os.path.join(pnpm, "node_modules")
        if os.path.isdir(pnpm_nm):
            for entry in ("@img", "sharp", "detect-libc"):
                p = os.path.join(pnpm_nm, entry)
                if os.path.isdir(p):
                    shutil.rmtree(p)


def prepare_web_dist():
    """组装平台无关的前端分发目录 build/web/（Next.js standalone 服务器）。

    结构：server.js + package.json + node_modules/ + .next/（server 与 static）+
    public/ + start.js；运行入口：node web/start.js（读锁文件 / 自动拉起后端）。
    """
    web_target = os.path.join(BUILD_DIR, "web")
    if os.path.isdir(web_target):
        shutil.rmtree(web_target)
    os.makedirs(web_target)

    # standalone 根内容（server.js、package.json、node_modules、.next/server 等）
    shutil.copytree(WEB_STANDALONE, web_target, dirs_exist_ok=True)
    # 客户端静态资源与公共资源
    shutil.copytree(os.path.join(WEB_DIR, ".next", "static"),
                    os.path.join(web_target, ".next", "static"), dirs_exist_ok=True)
    if os.path.isdir(os.path.join(WEB_DIR, "public")):
        shutil.copytree(os.path.join(WEB_DIR, "public"),
                        os.path.join(web_target, "public"), dirs_exist_ok=True)
    # 补齐 pnpm trace 遗漏的 next 运行时依赖，并移除平台特定 native 依赖
    patch_standalone_deps(web_target)
    strip_native_deps(web_target)
    # 前端启动器（读锁文件 / 拉起后端 / 启动 server.js）
    shutil.copy2(os.path.join(WEB_DIR, "start.js"), os.path.join(web_target, "start.js"))


def build_platforms(targets):
    """构建指定平台列表：前端（平台无关）组装一次，后端按平台逐个交叉编译。"""
    build_web()
    prepare_web_dist()
    print("\n✅ 前端产物：build/web/  （运行：cd build && node web/start.js）")
    for name in targets:
        p = PLATFORMS[name]
        print("\n[%s] %s" % (p.name, p.description))
        build_backend(p)
        print("   ✅ %s" % os.path.join(BUILD_DIR, "backend", p.name, binary_name(p)))

    print("\n✅ 构建完成，产物目录：")
    print("   build/web/                前端（平台无关，仅需 Node.js）")
    for name in targets:
        p = PLATFORMS[name]
        print("   build/backend/%s/  （后端，与 build/web 同级组合成可分发单元）" % name)


def ensure_build(platform):
    """确保某平台构建产物存在（存在则复用，缺失则先构建）。"""
    backend_dir = os.path.join(BUILD_DIR, "backend", platform.name)
    binary = os.path.join(backend_dir, binary_name(platform))
    web_server = os.path.join(BUILD_DIR, "web", "server.js")
    if os.path.isfile(binary) and os.path.isfile(web_server):
        print("   复用已有构建产物：build/web/ + build/backend/%s/" % platform.name)
        return
    print("   构建产物缺失，先构建 %s ..." % platform.name)
    if not os.path.isdir(WEB_STANDALONE):
        build_web()
    if not os.path.isfile(os.path.join(BUILD_DIR, "web", "server.js")):
        prepare_web_dist()
    build_backend(platform)


# ============================================================
# 打包通用工具
# ============================================================

def strip_copy(src, dst):
    """拷贝二进制并去除调试符号（减小安装包体积）。"""
    shutil.copy2(src, dst)
    os.chmod(dst, 0o755)
    if shutil.which("strip"):
        try:
            run(["strip", "--strip-unneeded", dst], quiet=True)
        except subprocess.CalledProcessError:
            pass  # 无法 strip 时保留原始副本


def write_web_entry(path, start_js):
    """写 /usr/bin 下的前端入口脚本（执行 node <start.js>，保持用户工作目录）。"""
    content = (
        "#!/bin/sh\n"
        "# 文件空间前端入口：启动 Next.js 前端（读后端锁文件，未启动则自动拉起）\n"
        'exec node %s "$@"\n' % start_js
    )
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    os.chmod(path, 0o755)


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
        "Exec=filespace-web\n"
        "Icon=filespace\n"
        "Terminal=true\n"
        "Categories=Network;FileTransfer;\n"
    )
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)


def install_linux_files(staging_usr_share, icon):
    """组装 Linux 安装布局（usr/share/ 下）：前端 web/ + 桌面入口 + 图标。"""
    web_target = os.path.join(staging_usr_share, "filespace", "web")
    shutil.copytree(os.path.join(BUILD_DIR, "web"), web_target)

    desktop = os.path.join(staging_usr_share, "applications", "filespace.desktop")
    os.makedirs(os.path.dirname(desktop), exist_ok=True)
    write_desktop(desktop)

    if icon:
        icon_target = os.path.join(staging_usr_share, "icons", "hicolor",
                                   "256x256", "apps", "filespace.png")
        os.makedirs(os.path.dirname(icon_target), exist_ok=True)
        shutil.copy2(icon, icon_target)


# ============================================================
# Windows 打包
# ============================================================

def pack_windows(platform, version):
    """Windows：msitools 生成 .msi 安装包。"""
    print("\n[打包] Windows %s" % platform.description)
    ensure_build(platform)
    out_dir = os.path.join(PACKAGES_DIR, "windows")
    os.makedirs(out_dir, exist_ok=True)

    if shutil.which("wixl") and shutil.which("wixl-heat"):
        msi_out = os.path.join(out_dir, "FileSpace-%s.msi" % version)
        try:
            _msi_installer(platform, version, msi_out)
            print("   ✅ %s" % msi_out)
        except subprocess.CalledProcessError as e:
            print("   ⚠️ .msi 打包失败：%s" % e)
    else:
        print("   ⚠️ 跳过 .msi：%s" % TOOL_HINTS["wixl"])


def _extract_balanced_block(text, start_pat):
    """从 start_pat 匹配位置起，用 <Directory>/</Directory> 深度匹配提取完整块。"""
    m = re.search(start_pat, text)
    if not m:
        return None
    start = m.start()
    depth = 0
    pos = start
    tag_re = re.compile(r"<(/?)Directory\b[^>]*>")
    while pos < len(text):
        t = tag_re.search(text, pos)
        if not t:
            break
        depth += -1 if t.group(1) == "/" else 1
        pos = t.end()
        if depth == 0:
            return text[start:pos]
    return None


def _msi_installer(platform, version, msi_out):
    """用 msitools（wixl-heat + wixl）生成 Windows .msi。

    wixl-heat 从 stdin 读文件列表生成 XML 片段（会为前缀目录生成一个
    Name="" 的空目录，wixl 无法处理，故只提取 Name="web" 的目录树，
    与 filespace.exe 一起平铺到 INSTALLFOLDER 下组成单树 wxs）。
    """
    require_tool("wixl")
    require_tool("wixl-heat")
    # 前端平台无关（build/web），后端按平台（build/backend/<平台>/）；SourceDir 统一指向 build
    web_dir = os.path.join(BUILD_DIR, "web")
    exe_rel = os.path.join("backend", platform.name, binary_name(platform))
    work = os.path.join(PACKAGES_DIR, "windows")
    os.makedirs(work, exist_ok=True)

    # 1) 收集 web/ 下全部文件，交给 wixl-heat 生成目录树片段
    files = []
    for root, _, names in os.walk(web_dir):
        for n in sorted(names):
            files.append(os.path.join(root, n))
    proc = subprocess.run(
        ["wixl-heat", "-p", BUILD_DIR, "--component-group", "Files",
         "--directory-ref", "INSTALLFOLDER"],
        input="\n".join(files) + "\n", text=True, capture_output=True)
    if proc.returncode != 0:
        sys.exit("错误：wixl-heat 失败：%s" % proc.stderr.strip())
    heat = proc.stdout.replace('Source="SourceDir//', 'Source="$(var.SourceDir)/')

    # 2) 提取 web 目录树与 ComponentRef 列表
    web_tree = _extract_balanced_block(heat, r'<Directory [^>]*Name="web">')
    cg = re.search(r'<ComponentGroup Id="Files">(.*?)</ComponentGroup>', heat, re.S)
    if not web_tree or not cg:
        sys.exit("错误：wixl-heat 输出格式异常，请检查 msitools 版本")
    refs = cg.group(1)

    # 3) 组装单树 .wxs（web 树 + filespace.exe 平铺在 INSTALLFOLDER 下）
    prod_id = str(uuid.uuid4()).upper()
    upgrade_code = str(uuid.uuid4()).upper()
    wxs = os.path.join(work, "filespace.wxs")
    with open(wxs, "w", encoding="utf-8") as f:
        f.write("""<?xml version="1.0" encoding="utf-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="%(prod_id)s" Name="%(name)s" Language="2052" Version="%(version)s"
           Manufacturer="FileSpace" UpgradeCode="%(upgrade_code)s">
    <Package InstallerVersion="200" Compressed="yes" InstallScope="perMachine"
             Description="局域网文件共享工具（P2P + mDNS）"/>
    <MajorUpgrade DowngradeErrorMessage="已安装更高版本，请先卸载旧版本。"/>
    <Media Id="1" Cabinet="product.cab" EmbedCab="yes"/>
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFiles64Folder">
        <Directory Id="INSTALLFOLDER" Name="FileSpace">
%(
web_tree)s
          <Component Id="cmpEXE" Guid="*">
            <File Id="filEXE" KeyPath="yes" Source="$(var.SourceDir)/%(exe_rel)s"/>
          </Component>
        </Directory>
      </Directory>
    </Directory>
    <Feature Id="Main" Title="%(name)s" Level="1">
%(refs)s
      <ComponentRef Id="cmpEXE"/>
    </Feature>
  </Product>
</Wix>
""" % {"prod_id": prod_id, "name": APP_NAME, "version": version,
       "upgrade_code": upgrade_code, "web_tree": web_tree, "refs": refs,
       "exe_rel": exe_rel.replace(os.sep, "/")})

    # 4) wixl 编译生成 .msi
    run(["wixl", "-o", os.path.abspath(msi_out), "-D", "SourceDir=" + BUILD_DIR, wxs])


# ============================================================
# Linux 打包
# ============================================================

def pack_linux(platform, version):
    """Linux：deb + pacman + AppImage。"""
    print("\n[打包] Linux %s" % platform.description)
    ensure_build(platform)
    out_dir = os.path.join(PACKAGES_DIR, "linux")
    os.makedirs(out_dir, exist_ok=True)
    icon = ensure_icon()

    if shutil.which("dpkg-deb"):
        deb_out = os.path.join(out_dir, "filespace_%s_amd64.deb" % version)
        _deb_package(platform, version, deb_out, icon)
        print("   ✅ %s" % deb_out)
    else:
        print("   ⚠️ 跳过 .deb：%s" % TOOL_HINTS["dpkg-deb"])

    if shutil.which("makepkg"):
        pkg_out = _pacman_package(platform, version, out_dir, icon)
        print("   ✅ %s" % pkg_out)
    else:
        print("   ⚠️ 跳过 pacman 包：%s" % TOOL_HINTS["makepkg"])

    if shutil.which("mksquashfs"):
        appimage_out = os.path.join(out_dir, "FileSpace-%s-x86_64.AppImage" % version)
        _appimage(platform, version, appimage_out, icon)
        print("   ✅ %s" % appimage_out)
    else:
        print("   ⚠️ 跳过 AppImage：%s" % TOOL_HINTS["mksquashfs"])


def _deb_package(platform, version, deb_out, icon):
    """用 dpkg-deb 构建 .deb。"""
    require_tool("dpkg-deb")
    staging = os.path.join(PACKAGES_DIR, "linux", "deb-root")
    if os.path.isdir(staging):
        shutil.rmtree(staging)

    backend_dir = os.path.join(BUILD_DIR, "backend", platform.name)

    # 后端二进制（strip 副本）+ 前端入口脚本
    usr_bin = os.path.join(staging, "usr", "bin")
    os.makedirs(usr_bin, exist_ok=True)
    strip_copy(os.path.join(backend_dir, binary_name(platform)),
               os.path.join(usr_bin, binary_name(platform)))
    write_web_entry(os.path.join(usr_bin, "filespace-web"),
                    "/usr/share/filespace/web/start.js")

    # web / 桌面入口 / 图标
    install_linux_files(os.path.join(staging, "usr", "share"), icon)

    # DEBIAN/control
    control = (
        "Package: filespace\n"
        "Version: %s\n"
        "Architecture: amd64\n"
        "Maintainer: %s\n"
        "Depends: libc6 (>= 2.34), nodejs\n"
        "Section: net\n"
        "Priority: optional\n"
        "Homepage: %s\n"
        "Description: 局域网文件共享工具（P2P + mDNS）\n"
        " 在任意文件夹下执行 filespace-web 即可共享该文件夹（自动拉起后端），\n"
        " 打开浏览器即可查看局域网内所有已共享的文件夹。\n"
    ) % (version, MAINTAINER, APP_URL)
    debian = os.path.join(staging, "DEBIAN")
    os.makedirs(debian, exist_ok=True)
    with open(os.path.join(debian, "control"), "w", encoding="utf-8") as f:
        f.write(control)

    run(["dpkg-deb", "--build", "--root-owner-group", staging, os.path.abspath(deb_out)])


def _pacman_package(platform, version, out_dir, icon):
    """用 makepkg + PKGBUILD 构建 pacman 包（.pkg.tar.zst）。"""
    require_tool("makepkg")
    work = os.path.join(out_dir, "pacman-build")
    if os.path.isdir(work):
        shutil.rmtree(work)
    os.makedirs(work)

    backend_dir = os.path.join(BUILD_DIR, "backend", platform.name)

    # 组装 source 归档（strip 二进制 + web + 桌面入口 + 图标）
    tar_root = os.path.join(work, "tar-root")
    os.makedirs(tar_root, exist_ok=True)
    strip_copy(os.path.join(backend_dir, binary_name(platform)),
               os.path.join(tar_root, binary_name(platform)))
    write_web_entry(os.path.join(tar_root, "filespace-web"),
                    "/usr/share/filespace/web/start.js")
    web_target = os.path.join(tar_root, "web")
    shutil.copytree(os.path.join(BUILD_DIR, "web"), web_target)
    write_desktop(os.path.join(tar_root, "filespace.desktop"))
    if icon:
        shutil.copy2(icon, os.path.join(tar_root, "filespace.png"))

    tarball = "filespace-%s-linux.tar.gz" % version
    run(["tar", "-czf", os.path.join(work, tarball), "-C", tar_root, "."])

    pkgbuild = """# Maintainer: %s
pkgname=filespace
pkgver=%s
pkgrel=1
pkgdesc="局域网文件共享工具（P2P + mDNS）"
arch=('x86_64')
url="%s"
license=('custom')
depends=('glibc' 'nodejs')
source=("%s")
sha256sums=('SKIP')

package() {
  cd "${srcdir}"
  install -Dm755 filespace "${pkgdir}/usr/bin/filespace"
  install -Dm755 filespace-web "${pkgdir}/usr/bin/filespace-web"
  install -dm755 "${pkgdir}/usr/share/filespace"
  cp -r web "${pkgdir}/usr/share/filespace/"
  install -Dm644 filespace.desktop "${pkgdir}/usr/share/applications/filespace.desktop"
  install -Dm644 filespace.png "${pkgdir}/usr/share/icons/hicolor/256x256/apps/filespace.png"
}
""" % (MAINTAINER, version, APP_URL, tarball)
    with open(os.path.join(work, "PKGBUILD"), "w", encoding="utf-8") as f:
        f.write(pkgbuild)

    run(["makepkg", "-f", "--noconfirm"], cwd=work)

    pkgs = [n for n in os.listdir(work) if n.endswith(".pkg.tar.zst")]
    if not pkgs:
        sys.exit("错误：makepkg 未产出 .pkg.tar.zst")
    pkg_out = os.path.join(out_dir, pkgs[0])
    shutil.move(os.path.join(work, pkgs[0]), pkg_out)
    return pkg_out


def _appimage(platform, version, appimage_out, icon):
    """手动构建 AppImage（mksquashfs + type2 runtime 拼接，无需 appimagetool/网络）。

    AppImage = ELF runtime + squashfs（AppDir 压缩镜像）。runtime 缓存于
    build/tools/appimage-cache/runtime-x86_64，缺失时自动下载。
    """
    require_tool("mksquashfs")
    appdir = os.path.join(PACKAGES_DIR, "linux", "FileSpace.AppDir")
    if os.path.isdir(appdir):
        shutil.rmtree(appdir)
    os.makedirs(appdir)

    backend_dir = os.path.join(BUILD_DIR, "backend", platform.name)

    # AppRun：不改变用户工作目录（共享的是调用方 cwd），启动 Next.js 前端
    #（web/start.js 会自动拉起同目录下的 filespace 后端；需系统安装 nodejs）
    apprun = os.path.join(appdir, "AppRun")
    with open(apprun, "w", encoding="utf-8") as f:
        f.write("#!/bin/sh\n"
                "# 文件空间 AppImage 启动脚本：资源在 AppDir 内，保持用户当前工作目录\n"
                'SELF="$(readlink -f "$0")"\n'
                'exec node "$(dirname "$SELF")/web/start.js" "$@"\n')
    os.chmod(apprun, 0o755)

    strip_copy(os.path.join(backend_dir, binary_name(platform)),
               os.path.join(appdir, binary_name(platform)))
    shutil.copytree(os.path.join(BUILD_DIR, "web"),
                    os.path.join(appdir, "web"))
    write_desktop(os.path.join(appdir, "filespace.desktop"))
    if icon:
        shutil.copy2(icon, os.path.join(appdir, "filespace.png"))

    # 1) 准备 type2 runtime（本地缓存优先，缺失则下载）
    cache_home = os.path.join(BUILD_DIR, "tools", "appimage-cache")
    os.makedirs(cache_home, exist_ok=True)
    runtime = os.path.join(cache_home, "runtime-x86_64")
    if not os.path.isfile(runtime):
        print("   下载 AppImage runtime ...")
        try:
            run(["curl", "-sL", "-m", "300", "--retry", "2",
                 "-o", runtime, APPIMAGE_RUNTIME_URL], quiet=True)
        except subprocess.CalledProcessError:
            sys.exit("错误：AppImage runtime 下载失败，可手动下载到 %s 后重试" % runtime)

    # 2) 压缩 AppDir 为 squashfs
    squash = os.path.join(PACKAGES_DIR, "linux", "appdir.squashfs")
    if os.path.isfile(squash):
        os.remove(squash)
    run(["mksquashfs", os.path.abspath(appdir), os.path.abspath(squash),
         "-root-owned", "-noappend"])

    # 3) runtime + squashfs 拼接为 AppImage
    with open(os.path.abspath(appimage_out), "wb") as out:
        with open(runtime, "rb") as f:
            out.write(f.read())
        with open(squash, "rb") as f:
            out.write(f.read())
    os.chmod(os.path.abspath(appimage_out), 0o755)
    os.remove(squash)


# ============================================================
# 打包入口
# ============================================================

# 可打包平台 → 打包函数（其余平台仅编译产物）
PACKERS = {
    "windows": pack_windows,
    "linux": pack_linux,
}


def do_pack(targets):
    """按平台打包安装包（windows / linux），其余平台仅构建产物。"""
    version = read_version()
    print("版本：%s" % version)
    packed = []
    for name in targets:
        p = PLATFORMS[name]
        if name in PACKERS:
            PACKERS[name](p, version)
            packed.append(name)
        else:
            print("\n⏭️  %s 暂无安装包格式（仅编译产物）" % p.description)

    if packed:
        print("\n✅ 打包完成，安装包目录：build/packages/")
        exts = (".msi", ".deb", ".pkg.tar.zst", ".AppImage")
        for name in packed:
            d = os.path.join(PACKAGES_DIR, name)
            if os.path.isdir(d):
                for f in sorted(os.listdir(d)):
                    p = os.path.join(d, f)
                    if os.path.isfile(p) and p.endswith(exts):
                        print("   %s" % p)


# ============================================================
# 清理 / 列表 / 主入口
# ============================================================

def clean():
    """清理构建产物与安装包，保留 build/ 下的 Go 构建缓存。"""
    print("==> 清理构建产物 ...")
    web_dir = os.path.join(BUILD_DIR, "web")
    if os.path.isdir(web_dir):
        shutil.rmtree(web_dir)
        print("   已删除 %s" % web_dir)
    backend_dir = os.path.join(BUILD_DIR, "backend")
    if os.path.isdir(backend_dir):
        shutil.rmtree(backend_dir)
        print("   已删除 %s" % backend_dir)
    # 旧版布局（build/<平台>/）清理
    for name in PLATFORMS:
        d = os.path.join(BUILD_DIR, name)
        if os.path.isdir(d):
            shutil.rmtree(d)
            print("   已删除 %s" % d)
    if os.path.isdir(WEB_STANDALONE):
        shutil.rmtree(WEB_STANDALONE)
        print("   已删除 %s" % WEB_STANDALONE)
    if os.path.isdir(PACKAGES_DIR):
        shutil.rmtree(PACKAGES_DIR)
        print("   已删除 %s" % PACKAGES_DIR)
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
