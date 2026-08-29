'use client'

import React, {Suspense, useEffect, useState} from 'react';
import {Alert, App as AntdApp, Breadcrumb, Button, Space, Spin, Table, Tooltip} from 'antd';
import type {ColumnsType} from 'antd/es/table';
import {
  DownloadOutlined,
  EyeOutlined,
  FileOutlined,
  FolderOutlined,
  FolderOpenOutlined,
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
  fetchNode,
  fetchPeers,
  fetchTree,
  openFile,
  type ApiFileInfo,
  type ApiFolderInfo,
} from '../_lib/api';

/**
 * 文件夹文件浏览页。
 * 路由为静态的 /folders?folderId=xxx（output:export 下动态路径段无法预渲染），
 * 通过查询参数定位共享文件夹。
 *
 * 本机文件夹（本机管理页进入）：文件无需下载/预览，操作为「打开」
 * （调用本机后端用系统默认应用打开）；远程文件夹：下载 + 预览。
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
    const {message} = AntdApp.useApp();
    const searchParams = useSearchParams();
    const folderId = searchParams.get('folderId') ?? '';
    const [folder, setFolder] = useState<ApiFolderInfo | null>(null);
    const [hostname, setHostname] = useState('');
    // 是否为本机共享文件夹：本机文件操作为「打开」，远程为「下载 / 预览」
    const [isLocal, setIsLocal] = useState(false);
    // 文件夹所属节点的 API 基地址：本机为空字符串（同源/反代），远端为 http://<ip>:<port>
    const [remoteBase, setRemoteBase] = useState('');
    const [path, setPath] = useState('');
    const [entries, setEntries] = useState<ApiFileInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [previewFile, setPreviewFile] = useState<ApiFileInfo | null>(null);
    // 正在打开的本地文件路径（避免重复点击）
    const [opening, setOpening] = useState<string | null>(null);

    // 加载文件夹信息（名称 / 磁盘路径 / 所属主机名 / 是否本机）
    useEffect(() => {
        if (!folderId) return;
        let cancelled = false;
        async function load() {
            try {
                const [node, folders, peers] = await Promise.all([
                    fetchNode(),
                    fetchFolders(),
                    fetchPeers(),
                ]);
                if (cancelled) return;
                // 先在本机共享的文件夹中查找，未命中再到 mDNS 发现的节点中查找。
                // 远端节点上的文件夹需通过该节点自己的后端直连（本地后端不认识其 id）。
                let found = folders.find((f) => f.id === folderId);
                let host = node.hostname;
                let base = '';
                let local = true;
                if (!found) {
                    for (const peer of peers) {
                        const f = peer.folders.find((x) => x.id === folderId);
                        if (f) {
                            found = f;
                            host = peer.node.hostname;
                            base = peer.node.listenAddr ? `http://${peer.node.listenAddr}` : '';
                            local = false;
                            break;
                        }
                    }
                }
                if (!found) {
                    setError('文件夹不存在或已移除');
                    return;
                }
                setFolder(found);
                setHostname(host);
                setIsLocal(local);
                setRemoteBase(base);
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
                const list = await fetchTree(folderId, path, remoteBase);
                if (!cancelled) setEntries(list);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, [folderId, path, remoteBase]);

    /** 本机文件：用系统默认应用打开（后端仅允许本机回环调用） */
    const handleOpen = async (record: ApiFileInfo) => {
        setOpening(record.path);
        try {
            await openFile(folderId, record.path);
            message.success(`已调用系统默认应用打开：${record.name}`);
        } catch (e) {
            message.error(e instanceof Error ? e.message : '打开失败');
        } finally {
            setOpening(null);
        }
    };

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
                record.isDir ? null : isLocal ? (
                    // 本机文件：用系统默认应用打开，无需下载 / 预览
                    <Tooltip title="使用系统默认应用打开">
                        <Button
                            type="link"
                            size="small"
                            loading={opening === record.path}
                            icon={<FolderOpenOutlined/>}
                            onClick={() => handleOpen(record)}
                        >
                            打开
                        </Button>
                    </Tooltip>
                ) : (
                    <Space size={4}>
                        <Tooltip title="下载">
                            <Button
                                type="link"
                                size="small"
                                icon={<DownloadOutlined/>}
                                href={downloadUrl(folderId, record.path, remoteBase)}
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
                title="加载失败"
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

    // 页面级面包屑：本机文件夹回到「本机管理」，远程文件夹回到「主机列表 > 主机名」
    const pageBreadcrumb =
        folder && (hostname || isLocal)
            ? isLocal
                ? [
                      {title: '本机管理', href: '/local'},
                      {title: folder.name},
                  ]
                : [
                      {title: '主机列表', href: '/'},
                      {title: hostname},
                      {title: folder.name},
                  ]
            : undefined;

    return (
        <AppShell
            wide
            title="文件夹"
            breadcrumb={pageBreadcrumb}
        >
            <div className="mb-4 flex items-center justify-between">
                <Breadcrumb items={breadcrumbItems}/>
                <Space>
                    {folder && (
                        <span className="text-xs text-neutral-500 dark:text-neutral-400">
                            磁盘路径：{folder.path}
                        </span>
                    )}
                    <Link href={isLocal ? '/local' : '/'}>
                        <Button size="small" icon={<HomeOutlined/>}>
                            {isLocal ? '返回本机管理' : '返回主机列表'}
                        </Button>
                    </Link>
                </Space>
            </div>
            {content}
            {/* 仅远程文件夹提供预览弹窗 */}
            {!isLocal && (
                <FilePreview
                    open={previewFile !== null}
                    onClose={() => setPreviewFile(null)}
                    fileUrl={previewFile ? downloadUrl(folderId, previewFile.path, remoteBase) : ''}
                    fileName={previewFile?.name ?? ''}
                />
            )}
        </AppShell>
    );
}
