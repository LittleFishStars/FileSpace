'use client'

import React, {useEffect, useState} from 'react';
import {Button, Drawer, Spin} from 'antd';
import {DownloadOutlined} from '@ant-design/icons';
import type {FileViewerProps} from '@smazeeapps/file-viewer';
import {useTheme} from './app_theme';

/**
 * 二进制格式（FileViewer 不支持、无法作为文本预览）→ 直接下载。
 * 注意排除 FileViewer 已支持的格式（图片 / 视频 / PDF / Office 等）。
 */
const BINARY_RE = /\.(zip|tar|gz|tgz|bz2|xz|7z|rar|exe|dll|so|dylib|bin|iso|deb|rpm|apk|msi|class|jar|war|pyc|o|a|db|sqlite|sqlite3|parquet|pdb|dat|pak)$/i;

/**
 * 文件预览弹窗。
 * - PDF   → 浏览器原生查看器（iframe 嵌入，零依赖、无需联网）
 * - 二进制 → 下载提示
 * - 其他  → @smazeeapps/file-viewer（Office / 代码 / 文本 / 图片 / 视频等）
 */
export default function FilePreview({
    open,
    onClose,
    fileUrl,
    fileName,
}: {
    open: boolean;
    onClose: () => void;
    fileUrl: string;
    fileName: string;
}) {
    const {isDark} = useTheme();
    const [Viewer, setViewer] = useState<React.ComponentType<FileViewerProps> | null>(null);

    // 懒加载 FileViewer（组件内部使用浏览器 API 与动态分块，避免 SSR 打包执行）
    useEffect(() => {
        let cancelled = false;
        import('@smazeeapps/file-viewer').then((m) => {
            if (!cancelled) setViewer(() => m.FileViewer);
        });
        return () => {
            cancelled = true;
        };
    }, []);

    let inner: React.ReactNode;
    if (/\.pdf$/i.test(fileName)) {
        // PDF：浏览器原生查看器（iframe 嵌入）
        inner = (
            <iframe
                key={fileUrl}
                src={fileUrl}
                title={fileName}
                style={{width: '100%', height: '100%', border: 'none'}}
            />
        );
    } else if (BINARY_RE.test(fileName)) {
        // 二进制格式：提示下载
        inner = <UnsupportedPreview fileUrl={fileUrl} fileName={fileName}/>;
    } else if (Viewer) {
        // 其他所有格式（含未知非二进制）交给 FileViewer
        inner = (
            <div style={{height: '100%', overflow: 'auto'}}>
                {/* key=fileUrl：切换文件时重建，避免复用上个文件的渲染状态 */}
                <Viewer
                    key={fileUrl}
                    src={fileUrl}
                    fileName={fileName}
                    theme={isDark ? 'dark' : 'light'}
                    height="100%"
                />
            </div>
        );
    } else {
        inner = (
            <div style={{height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
                <Spin size="large"/>
            </div>
        );
    }

    return (
        <Drawer
            title={fileName}
            placement="right"
            size="80vw"
            onClose={onClose}
            open={open}
            styles={{body: {padding: 0, overflow: 'hidden'}}}
        >
            {open && inner}
        </Drawer>
    );
}

/** 二进制 / 不支持格式：下载提示 */
function UnsupportedPreview({fileUrl, fileName}: {fileUrl: string; fileName: string}) {
    return (
        <div
            style={{
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 16,
            }}
        >
            <span style={{color: 'rgba(128,128,128,0.8)'}}>
                二进制格式，暂不支持在线预览（{fileName}）
            </span>
            <Button type="primary" icon={<DownloadOutlined/>} href={fileUrl} download>
                下载文件
            </Button>
        </div>
    );
}
