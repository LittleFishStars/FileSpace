#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FileSpace 构建脚本 - 打包模块

负责 Windows（.msi）与 Linux（deb + pacman + AppImage）安装包的构建。
"""

import os
import shutil
import subprocess
import sys
import uuid

# 确保 scripts/ 目录在 sys.path，支持直接运行脚本
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from _build_common import (
    APP_NAME, APP_URL, APPIMAGE_RUNTIME_URL, BUILD_DIR,
    MAINTAINER, PACKAGES_DIR, PLATFORMS, TOOL_HINTS, binary_name, ensure_icon,
    read_version, require_tool, run, strip_copy, write_desktop,
)
from _build_compile import ensure_build


# ============================================================
# Windows 打包
# ============================================================

def pack_windows(platform, version):
    """Windows：msitools 生成 .msi 安装包。"""
    print("\n[打包] Windows %s" % platform.description)
    ensure_build(platform)
    out_dir = os.path.join(PACKAGES_DIR, "windows")
    os.makedirs(out_dir, exist_ok=True)

    if shutil.which("wixl"):
        msi_out = os.path.join(out_dir, "FileSpace-%s.msi" % version)
        try:
            _msi_installer(platform, version, msi_out)
            print("   ✅ %s" % msi_out)
        except subprocess.CalledProcessError as e:
            print("   ⚠️ .msi 打包失败：%s" % e)
    else:
        print("   ⚠️ 跳过 .msi：%s" % TOOL_HINTS["wixl"])


def _msi_installer(platform, version, msi_out):
    """用 wixl 生成 Windows .msi。

    注意：不再使用 wixl-heat 生成目录树——msitools 0.106 的 wixl-heat 对
    文件直接位于 -p 前缀目录下的情况会生成两层 Name="" 空目录，wixl 编译时
    报 libmsi_query_execute（无更多信息）。这里由脚本直接为每个文件生成
    Component/File 元素，避免依赖 wixl-heat 的目录树输出。
    """
    require_tool("wixl")
    platform_dir = os.path.join(BUILD_DIR, platform.name)
    exe_path = os.path.join(platform_dir, binary_name(platform))
    work = os.path.join(PACKAGES_DIR, "windows")
    os.makedirs(work, exist_ok=True)

    # 1) 为每个后端二进制生成 Component/File 元素（相对 platform_dir 的路径）
    files = [exe_path]
    comps, refs = [], []
    for i, f in enumerate(files):
        rel = os.path.relpath(f, platform_dir).replace("\\", "/")
        cid = "cmpMain" if i == 0 else "cmp%d" % i
        fid = "filMain" if i == 0 else "fil%d" % i
        comps.append(
            '      <Component Id="%s" Guid="*">\n'
            '        <File Id="%s" KeyPath="yes" Source="$(var.SourceDir)/%s"/>\n'
            '      </Component>' % (cid, fid, rel))
        refs.append('      <ComponentRef Id="%s"/>' % cid)
    comps_xml = "\n".join(comps)
    refs_xml = "\n".join(refs)

    # 2) 组装单树 .wxs
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
%(comps_xml)s
        </Directory>
      </Directory>
    </Directory>
    <Feature Id="Main" Title="%(name)s" Level="1">
%(refs_xml)s
    </Feature>
  </Product>
</Wix>
""" % {"prod_id": prod_id, "name": APP_NAME, "version": version,
       "upgrade_code": upgrade_code, "comps_xml": comps_xml, "refs_xml": refs_xml})

    # 3) wixl 编译生成 .msi
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
