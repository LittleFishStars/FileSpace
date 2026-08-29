'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Modal, Spin} from 'antd';
import {detectFileType} from '@smazeeapps/file-viewer';
import type {FileViewerProps} from '@smazeeapps/file-viewer';
import {useTheme} from './app_theme';

/**
 * 关键修复：阻止 prismjs 的自动 highlightAll 破坏 FileViewer 已渲染的逐行代码结构。
 *
 * prismjs 加载后（默认 manual=false）会在下一帧自动执行 Prism.highlightAll()，
 * 扫描页面中所有 code.language-* 元素并调用 highlightElement 重写其 innerHTML。
 * FileViewer 的 <code class="fv-code-viewer language-go"> 会被命中：
 * - 给 <pre> 加上 language-* 与 tabindex
 * - 读取所有行 span 的 textContent（行号被拼入、换行丢失），把逐行结构拍平成一行
 *
 * 必须在 FileViewer 内部 import('prismjs') 之前设置 manual=true，
 * prism 初始化时读取 window.Prism.manual（第 56 行），为 true 则跳过自动高亮
 * （CodeViewer 主动调用的 Prism.highlight() 不受影响）。
 */
if (typeof window !== 'undefined') {
    (window as unknown as {Prism?: {manual: boolean}}).Prism = {manual: true};
}

/**
 * 文件预览弹窗。
 * 渲染路由基于包的 detectFileType（与 FileViewer 内部支持列表同步）：
 * - pdf       → 浏览器原生查看器（iframe）
 * - 非 unknown → @smazeeapps/file-viewer（Office / 代码 / 文本 / 图片 / 视频等）
 * - unknown   → 纯文本显示（FileViewer 不支持，但后端已确认为可预览的文本）
 *
 * 二进制文件由后端内容嗅探（previewable=false）处理，按钮已隐藏，不会进入本弹窗。
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
    // 小屏（≤768px）时全屏显示
    const [isMobile, setIsMobile] = useState(false);

    useEffect(() => {
        const mq = window.matchMedia('(max-width: 768px)');
        const update = () => setIsMobile(mq.matches);
        update();
        mq.addEventListener('change', update);
        return () => mq.removeEventListener('change', update);
    }, []);

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

    const kind = detectFileType(fileName);

    let inner: React.ReactNode;
    if (kind === 'pdf') {
        // PDF：浏览器原生查看器（iframe 嵌入）
        inner = (
            <iframe
                key={fileUrl}
                src={fileUrl}
                title={fileName}
                style={{width: '100%', height: '100%', border: 'none'}}
            />
        );
    } else if (kind !== 'unknown' && Viewer) {
        // FileViewer 支持的格式（Office / 代码 / 文本 / 图片 / 视频等）
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
    } else if (kind !== 'unknown') {
        // FileViewer 还在懒加载中
        inner = (
            <div style={{height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
                <Spin size="large"/>
            </div>
        );
    } else {
        // FileViewer 不支持的格式（非二进制）：纯文本显示
        inner = <TextViewer key={fileUrl} fileUrl={fileUrl}/>;
    }

    return (
        <Modal
            open={open}
            onCancel={onClose}
            title={fileName}
            footer={null}
            width={isMobile ? '100vw' : '80vw'}
            centered={!isMobile}
            styles={{
                // 小屏：铺满全屏；大屏：居中大弹窗
                root: isMobile ? {maxWidth: '100vw'} : undefined,
                wrapper: isMobile ? {top: 0, padding: 0} : undefined,
                container: isMobile
                    ? {maxWidth: '100vw', width: '100vw', height: '100vh', margin: 0, top: 0, borderRadius: 0}
                    : undefined,
                header: {marginBottom: 0, padding: '12px 20px'},
                body: {
                    padding: 0,
                    height: isMobile ? 'calc(100vh - 49px)' : '72vh',
                    overflow: 'hidden',
                },
                footer: {display: 'none'},
            }}
        >
            {open && inner}
        </Modal>
    );
}

/**
 * 纯文本查看器：FileViewer 不支持的格式（非二进制）以文本展示。
 * no-store 拉取内容，原生 <pre> 逐行显示，带行号。
 */
function TextViewer({fileUrl}: {fileUrl: string}) {
    const {isDark} = useTheme();
    const [content, setContent] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const res = await fetch(fileUrl, {cache: 'no-store'});
                if (!res.ok) {
                    throw new Error(`下载失败：${res.status}`);
                }
                const text = await res.text();
                if (!cancelled) setContent(text);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [fileUrl]);

    if (error) {
        return (
            <div style={{height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
                <Alert type="error" showIcon title="预览失败" description={error}/>
            </div>
        );
    }
    if (content === null) {
        return (
            <div style={{height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
                <Spin size="large"/>
            </div>
        );
    }

    const bg = isDark ? '#111827' : '#ffffff';
    const fg = isDark ? '#d4d4d4' : '#24292f';
    const lineFg = isDark ? '#6b7280' : '#8c959f';
    const font = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';
    const lineCount = content.split('\n').length;

    return (
        <div
            style={{
                height: '100%',
                overflow: 'auto',
                background: bg,
                display: 'flex',
                fontSize: 14,
                lineHeight: 1.65,
                fontFamily: font,
            }}
        >
            {/* 行号列 */}
            <pre
                aria-hidden
                style={{
                    margin: 0,
                    padding: '16px 8px 16px 16px',
                    color: lineFg,
                    textAlign: 'right',
                    userSelect: 'none',
                    flexShrink: 0,
                    background: 'transparent',
                }}
            >
                {Array.from({length: lineCount}, (_, i) => i + 1).join('\n')}
            </pre>
            {/* 内容列：pre 原生保留换行 */}
            <pre
                style={{
                    margin: 0,
                    padding: '16px 16px 16px 8px',
                    color: fg,
                    whiteSpace: 'pre',
                    flex: 1,
                    minWidth: 0,
                    background: 'transparent',
                }}
            >
                {content}
            </pre>
        </div>
    );
}
