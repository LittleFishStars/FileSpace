'use client'

import React, {useEffect, useState} from 'react';
import {
    Alert,
    App as AntdApp,
    Badge,
    Button,
    Empty,
    Input,
    Modal,
    Popconfirm,
    Spin,
    Table,
    Tooltip,
} from 'antd';
import type {ColumnsType} from 'antd/es/table';
import {
    CloudServerOutlined,
    DeleteOutlined,
    EyeOutlined,
    FolderAddOutlined,
    FolderOpenOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import {ProCard} from '@ant-design/pro-components';
import AppShell from '../_components/app_shell';
import NodeInfoCard from '../_cards/node_info_card';
import {formatSize} from '../_cards/folder_card';
import {
    addFolders,
    fetchFolders,
    fetchNode,
    removeFolder,
    type ApiFolderInfo,
    type ApiNodeInfo,
} from '../_lib/api';

/**
 * 本机管理页（/local）。
 * 本机节点不出现在主机列表中，共享文件夹在本页管理：
 * 查看本机节点信息、添加 / 删除共享文件夹。
 * 添加 / 删除均需本机回环访问后端（后端强制校验），局域网其他机器无法通过此页修改本机共享。
 */

/** 把 RFC3339 时间显示为本地可读格式 */
function formatTime(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString('zh-CN', {hour12: false});
}

export default function LocalPage() {
    const {message} = AntdApp.useApp();
    const [node, setNode] = useState<ApiNodeInfo | null>(null);
    const [folders, setFolders] = useState<ApiFolderInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    // 添加共享文件夹弹窗状态
    const [addOpen, setAddOpen] = useState(false);
    const [addText, setAddText] = useState('');
    const [adding, setAdding] = useState(false);
    // 正在删除的文件夹 ID（避免重复点击）
    const [removing, setRemoving] = useState<string | null>(null);
    // 数据刷新计数：添加 / 删除成功后 +1 触发重新加载
    const [refresh, setRefresh] = useState(0);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [n, fs] = await Promise.all([fetchNode(), fetchFolders()]);
                if (cancelled) return;
                setNode(n);
                setFolders(fs);
                setError(null);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, [refresh]);

    /** 解析弹窗中输入的多行路径并添加共享 */
    const handleAdd = async () => {
        const paths = addText
            .split('\n')
            .map((p) => p.trim())
            .filter(Boolean);
        if (paths.length === 0) {
            message.warning('请输入至少一个目录路径');
            return;
        }
        setAdding(true);
        try {
            const added = await addFolders(paths);
            message.success(`已添加 ${added.length} 个共享文件夹`);
            setAddOpen(false);
            setAddText('');
            setRefresh((r) => r + 1);
        } catch (e) {
            message.error(e instanceof Error ? e.message : '添加失败');
        } finally {
            setAdding(false);
        }
    };

    /** 移除共享文件夹 */
    const handleRemove = async (id: string) => {
        setRemoving(id);
        try {
            await removeFolder(id);
            message.success('已移除共享文件夹');
            setRefresh((r) => r + 1);
        } catch (e) {
            message.error(e instanceof Error ? e.message : '移除失败');
        } finally {
            setRemoving(null);
        }
    };

    const columns: ColumnsType<ApiFolderInfo> = [
        {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            ellipsis: true,
            render: (_, record) => (
                <span className="flex items-center gap-2">
                    <FolderOpenOutlined className="text-amber-500"/>
                    <span className="font-medium">{record.name}</span>
                </span>
            ),
        },
        {
            title: '磁盘路径',
            dataIndex: 'path',
            key: 'path',
            ellipsis: true,
            render: (value: string) => (
                <Tooltip title={value}>
                    <span className="text-neutral-500 dark:text-neutral-400">{value}</span>
                </Tooltip>
            ),
        },
        {
            title: '文件数',
            dataIndex: 'fileCount',
            key: 'fileCount',
            width: 90,
            align: 'right' as const,
        },
        {
            title: '总大小',
            dataIndex: 'totalSize',
            key: 'totalSize',
            width: 110,
            align: 'right' as const,
            render: (value: number) => formatSize(value),
        },
        {
            title: '更新时间',
            dataIndex: 'updatedAt',
            key: 'updatedAt',
            width: 180,
            render: (value: string) => formatTime(value),
        },
        {
            title: '操作',
            key: 'action',
            width: 140,
            render: (_, record) => (
                <div className="flex items-center gap-1">
                    <Link href={`/folders?folderId=${record.id}`}>
                        <Button type="link" size="small" icon={<EyeOutlined/>}>
                            浏览
                        </Button>
                    </Link>
                    <Popconfirm
                        title="移除共享文件夹"
                        description={`确定不再共享「${record.name}」吗？`}
                        okText="移除"
                        cancelText="取消"
                        okButtonProps={{danger: true}}
                        onConfirm={() => handleRemove(record.id)}
                    >
                        <Button
                            type="link"
                            size="small"
                            danger
                            loading={removing === record.id}
                            icon={<DeleteOutlined/>}
                        >
                            删除
                        </Button>
                    </Popconfirm>
                </div>
            ),
        },
    ];

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (node === null || folders === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else {
        content = (
            <div className="flex flex-col gap-4">
                {/* 本机节点信息 */}
                <ProCard
                    bordered
                    headerBordered
                    title={
                        <span className="flex items-center gap-2 text-base font-semibold">
                            <CloudServerOutlined className="text-neutral-500 dark:text-neutral-400"/>
                            {node.hostname}
                        </span>
                    }
                    extra={
                        <Badge status="success" text="在线"/>
                    }
                    bodyStyle={{padding: 16}}
                >
                    <NodeInfoCard
                        info={{
                            ip: node.ip,
                            os: node.os,
                            softwareVersion: node.softwareVersion,
                            uptime: node.uptime,
                        }}
                    />
                    <div className="text-xs text-neutral-500 dark:text-neutral-400">
                        本机节点不出现在主机列表中，共享文件夹在此管理（仅本机可修改）。
                    </div>
                </ProCard>

                {/* 共享文件夹管理 */}
                <ProCard
                    bordered
                    headerBordered
                    title={
                        <span className="flex items-center gap-2 text-base font-semibold">
                            <FolderOpenOutlined className="text-amber-500 dark:text-amber-400"/>
                            共享文件夹
                            <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">
                                共 {folders.length} 个
                            </span>
                        </span>
                    }
                    extra={
                        <Button type="primary" icon={<FolderAddOutlined/>} onClick={() => setAddOpen(true)}>
                            添加共享文件夹
                        </Button>
                    }
                    bodyStyle={{padding: 16}}
                >
                    {folders.length === 0 ? (
                        <Empty description="暂无共享文件夹，点击右上角「添加共享文件夹」开始共享"/>
                    ) : (
                        <Table<ApiFolderInfo>
                            rowKey="id"
                            size="middle"
                            columns={columns}
                            dataSource={folders}
                            pagination={false}
                        />
                    )}
                </ProCard>
            </div>
        );
    }

    return (
        <AppShell title="本机管理">
            {content}

            {/* 添加共享文件夹弹窗：每行一个目录绝对路径 */}
            <Modal
                open={addOpen}
                title="添加共享文件夹"
                okText="添加"
                cancelText="取消"
                confirmLoading={adding}
                onOk={handleAdd}
                onCancel={() => {
                    setAddOpen(false);
                    setAddText('');
                }}
            >
                <div className="mb-2 text-sm text-neutral-500 dark:text-neutral-400">
                    请输入要共享的目录绝对路径，每行一个：
                </div>
                <Input.TextArea
                    value={addText}
                    onChange={(e) => setAddText(e.target.value)}
                    placeholder={'例如：\n/home/user/Documents\n/media/data'}
                    autoSize={{minRows: 3, maxRows: 8}}
                />
            </Modal>
        </AppShell>
    );
}
