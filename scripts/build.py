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
WEB_EXPORT = os.path.join(WEB_DIR, "out")     # pnpm build（output: export）的输出目录
EMBED_DIR = os.path.join(BACKEND_DIR, "cmd", "filespace", "web")  # go:embed 嵌入源目录

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
    """构建前端静态导出（output: 'export' → web/out/）。"""
    print("\n[前端] 构建静态导出 ...")
    run(["pnpm", "build"], cwd=WEB_DIR)
    if not os.path.isdir(WEB_EXPORT):
        sys.exit("错误：前端构建未产出 %s，请检查 web/ 构建配置" % WEB_EXPORT)


def prepare_embed_dir():
    """将前端静态导出拷贝到 go:embed 嵌入源目录（backend/cmd/filespace/web/）。"""
    print("\n[前端] 拷贝静态资源到嵌入源目录 ...")
    if os.path.isdir(EMBED_DIR):
        shutil.rmtree(EMBED_DIR)
    shutil.copytree(WEB_EXPORT, EMBED_DIR)
    print("   ✅ %s" % EMBED_DIR)


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
    """用 msitools（wixl-heat + wixl）生成 Windows .msi。"""
    require_tool("wixl")
    require_tool("wixl-heat")
    platform_dir = os.path.join(BUILD_DIR, platform.name)
    exe_path = os.path.join(platform_dir, binary_name(platform))
    work = os.path.join(PACKAGES_DIR, "windows")
    os.makedirs(work, exist_ok=True)

    # 1) 收集后端二进制文件，交给 wixl-heat 生成目录树片段
    files = [exe_path]
    proc = subprocess.run(
        ["wixl-heat", "-p", platform_dir, "--component-group", "Files",
         "--directory-ref", "INSTALLFOLDER"],
        input="\n".join(files) + "\n", text=True, capture_output=True)
    if proc.returncode != 0:
        sys.exit("错误：wixl-heat 失败：%s" % proc.stderr.strip())
    heat = proc.stdout.replace('Source="SourceDir//', 'Source="$(var.SourceDir)/')

    # 2) 提取 filespace.exe 目录树与 ComponentRef 列表
    exe_tree = _extract_balanced_block(heat, r'<Directory [^>]*Name="filespace">')
    if not exe_tree:
        # 回退：直接匹配第一个 Directory 块
        exe_tree = _extract_balanced_block(heat, r'<Directory [^>]*Name="[^"]*">')
    cg = re.search(r'<ComponentGroup Id="Files">(.*?)</ComponentGroup>', heat, re.S)
    if not exe_tree or not cg:
        sys.exit("错误：wixl-heat 输出格式异常，请检查 msitools 版本")
    refs = cg.group(1)

    # 3) 组装单树 .wxs
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
%(exe_tree)s
        </Directory>
      </Directory>
    </Directory>
    <Feature Id="Main" Title="%(name)s" Level="1">
%(refs)s
    </Feature>
  </Product>
</Wix>
""" % {"prod_id": prod_id, "name": APP_NAME, "version": version,
       "upgrade_code": upgrade_code, "exe_tree": exe_tree, "refs": refs})

    # 4) wixl 编译生成 .msi
    run(["wixl", "-o", os.path.abspath(msi_out), "-D", "SourceDir=" + platform_dir, wxs])


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

    platform_dir = os.path.join(BUILD_DIR, platform.name)

    # 后端二进制（strip 副本）
    usr_bin = os.path.join(staging, "usr", "bin")
    os.makedirs(usr_bin, exist_ok=True)
    strip_copy(os.path.join(platform_dir, binary_name(platform)),
               os.path.join(usr_bin, binary_name(platform)))

    # 桌面入口 + 图标
    desktop = os.path.join(staging, "usr", "share", "applications", "filespace.desktop")
    os.makedirs(os.path.dirname(desktop), exist_ok=True)
    write_desktop(desktop)

    if icon:
        icon_target = os.path.join(staging, "usr", "share", "icons", "hicolor",
                                   "256x256", "apps", "filespace.png")
        os.makedirs(os.path.dirname(icon_target), exist_ok=True)
        shutil.copy2(icon, icon_target)

    # DEBIAN/control
    control = (
        "Package: filespace\n"
        "Version: %s\n"
        "Architecture: amd64\n"
        "Maintainer: %s\n"
        "Depends: libc6 (>= 2.34)\n"
        "Section: net\n"
        "Priority: optional\n"
        "Homepage: %s\n"
        "Description: 局域网文件共享工具（P2P + mDNS）\n"
        " 在任意文件夹下执行 filespace --web 即可共享该文件夹（自动拉起后端），\n"
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

    platform_dir = os.path.join(BUILD_DIR, platform.name)

    # 组装 source 归档（strip 二进制 + 桌面入口 + 图标）
    tar_root = os.path.join(work, "tar-root")
    os.makedirs(tar_root, exist_ok=True)
    strip_copy(os.path.join(platform_dir, binary_name(platform)),
               os.path.join(tar_root, binary_name(platform)))
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
depends=('glibc')
source=("%s")
sha256sums=('SKIP')

package() {
  cd "${srcdir}"
  install -Dm755 filespace "${pkgdir}/usr/bin/filespace"
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

    platform_dir = os.path.join(BUILD_DIR, platform.name)

    # AppRun：启动 filespace --web（保持用户当前工作目录）
    apprun = os.path.join(appdir, "AppRun")
    with open(apprun, "w", encoding="utf-8") as f:
        f.write("#!/bin/sh\n"
                "# 文件空间 AppImage 启动脚本：保持用户当前工作目录\n"
                'SELF="$(readlink -f "$0")"\n'
                'exec "$(dirname "$SELF")/filespace" --web "$@"\n')
    os.chmod(apprun, 0o755)

    strip_copy(os.path.join(platform_dir, binary_name(platform)),
               os.path.join(appdir, binary_name(platform)))
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
