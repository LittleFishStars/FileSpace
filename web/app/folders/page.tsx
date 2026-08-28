'use client'

import React, {Suspense, useEffect, useState} from 'react';
import {Alert, Breadcrumb, Button, Space, Spin, Table, Tooltip} from 'antd';
import type {ColumnsType} from 'antd/es/table';
import {
  DownloadOutlined,
  EyeOutlined,
  FileOutlined,
  FolderOutlined,
  HomeOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import AppShell from '../_components/app_shell';
import FilePreview from '../_components/file_preview';
import {formatSize} from '../_cards/folder_card';
import {
  downloadUrl,
  fetchFolders,
  fetchTree,
  type ApiFileInfo,
  type ApiFolderInfo,
} from '../_lib/api';

/**
 * 文件夹文件浏览页。
 * 路由为静态的 /folders?folderId=xxx（output:export 下动态路径段无法预渲染），
 * 通过查询参数定位共享文件夹。
 */

/** 把 RFC3339 时间显示为本地可读格式 */
function formatTime(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString('zh-CN', {hour12: false});
}

export default function FolderPage() {
    return (
        <Suspense fallback={null}>
            <FolderBrowser/>
        </Suspense>
    );
}

function FolderBrowser() {
    const searchParams = useSearchParams();
    const folderId = searchParams.get('folderId') ?? '';
    const [folder, setFolder] = useState<ApiFolderInfo | null>(null);
    const [path, setPath] = useState('');
    const [entries, setEntries] = useState<ApiFileInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [previewFile, setPreviewFile] = useState<ApiFileInfo | null>(null);

    // 加载文件夹信息（名称 / 磁盘路径）
    useEffect(() => {
        if (!folderId) return;
        let cancelled = false;
        async function load() {
            try {
                const folders = await fetchFolders();
                if (cancelled) return;
                const found = folders.find((f) => f.id === folderId);
                if (!found) {
                    setError('文件夹不存在或已移除');
                    return;
                }
                setFolder(found);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, [folderId]);

    // 懒加载当前目录内容
    useEffect(() => {
        if (!folderId) return;
        let cancelled = false;
        async function load() {
            setEntries(null);
            setError(null);
            try {
                const list = await fetchTree(folderId, path);
                if (!cancelled) setEntries(list);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, [folderId, path]);

    // 面包屑：根目录 + 各级目录
    const segments = path ? path.split('/') : [];
    const breadcrumbItems = [
        {
            title: (
                <span
                    className="cursor-pointer text-neutral-600 hover:text-blue-500 dark:text-neutral-300"
                    onClick={() => setPath('')}
                >
                    根目录
                </span>
            ),
        },
        ...segments.map((seg, i) => ({
            title: (
                <span
                    className={
                        i === segments.length - 1
                            ? 'text-neutral-900 dark:text-neutral-100'
                            : 'cursor-pointer text-neutral-600 hover:text-blue-500 dark:text-neutral-300'
                    }
                    onClick={() => setPath(segments.slice(0, i + 1).join('/'))}
                >
                    {seg}
                </span>
            ),
        })),
    ];

    const columns: ColumnsType<ApiFileInfo> = [
        {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            ellipsis: true,
            render: (_, record) =>
                record.isDir ? (
                    <span
                        className="cursor-pointer text-blue-600 hover:underline dark:text-blue-400"
                        onClick={() => setPath(record.path)}
                    >
                        <FolderOutlined className="mr-2 text-amber-500"/>
                        {record.name}
                    </span>
                ) : (
                    <span>
                        <FileOutlined className="mr-2 text-neutral-400"/>
                        {record.name}
                    </span>
                ),
        },
        {
            title: '大小',
            dataIndex: 'size',
            key: 'size',
            width: 120,
            align: 'right' as const,
            render: (_, record) => (record.isDir ? '-' : formatSize(record.size)),
        },
        {
            title: '修改时间',
            dataIndex: 'modTime',
            key: 'modTime',
            width: 190,
            render: (value: string) => formatTime(value),
        },
        {
            title: '操作',
            key: 'action',
            width: 150,
            render: (_, record) =>
                record.isDir ? null : (
                    <Space size={4}>
                        <Tooltip title="下载">
                            <Button
                                type="link"
                                size="small"
                                icon={<DownloadOutlined/>}
                                href={downloadUrl(folderId, record.path)}
                                download
                            >
                                下载
                            </Button>
                        </Tooltip>
                        {/* 后端判断不可预览的文件（二进制等）不显示"查看"按钮 */}
                        {record.previewable && (
                            <Tooltip title="预览">
                                <Button
                                    type="link"
                                    size="small"
                                    icon={<EyeOutlined/>}
                                    onClick={() => setPreviewFile(record)}
                                >
                                    查看
                                </Button>
                            </Tooltip>
                        )}
                    </Space>
                ),
        },
    ];

    let content: React.ReactNode;
    if (error) {
        content = (
            <Alert
                type="error"
                showIcon
                message="加载失败"
                description={error}
                action={
                    <Button type="primary" size="small" onClick={() => setPath('')}>
                        返回根目录
                    </Button>
                }
            />
        );
    } else if (entries === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else {
        content = (
            <Table<ApiFileInfo>
                rowKey="path"
                size="middle"
                columns={columns}
                dataSource={entries}
                pagination={false}
            />
        );
    }

    return (
        <AppShell title={folder ? folder.name : '文件夹'} wide>
            <div className="mb-4 flex items-center justify-between">
                <Breadcrumb items={breadcrumbItems}/>
                <Space>
                    {folder && (
                        <span className="text-xs text-neutral-500 dark:text-neutral-400">
                            磁盘路径：{folder.path}
                        </span>
                    )}
                    <Link href="/">
                        <Button size="small" icon={<HomeOutlined/>}>
                            返回主机列表
                        </Button>
                    </Link>
                </Space>
            </div>
            {content}
            <FilePreview
                open={previewFile !== null}
                onClose={() => setPreviewFile(null)}
                fileUrl={previewFile ? downloadUrl(folderId, previewFile.path) : ''}
                fileName={previewFile?.name ?? ''}
            />
        </AppShell>
    );
}
