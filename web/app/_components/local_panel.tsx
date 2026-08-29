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
    LockOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import {ProCard} from '@ant-design/pro-components';
import NodeInfoCard from '../_cards/node_info_card';
import {formatSize} from '../_cards/folder_card';
import {
    addFolders,
    fetchFolders,
    fetchNode,
    pickDirectory,
    removeFolder,
    type ApiFolderInfo,
    type ApiNodeInfo,
} from '../_lib/api';

/**
 * 本机节点面板：本机节点信息 + 共享文件夹管理（添加 / 删除）。
 * 被主页「本机节点」选项卡与 /local 本机管理页复用。
 * 添加 / 删除均需本机回环访问后端（后端强制校验），局域网其他机器无法通过此面板修改本机共享。
 * 添加共享默认调用后端在本机弹出系统原生目录选择对话框（浏览器不暴露所选文件夹绝对路径，
 * 故由后端弹对话框）；系统选择器不可用时降级为手动输入绝对路径。
 */

/** 把 RFC3339 时间显示为本地可读格式 */
function formatTime(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString('zh-CN', {hour12: false});
}

export default function LocalPanel() {
    const {message} = AntdApp.useApp();
    const [node, setNode] = useState<ApiNodeInfo | null>(null);
    const [folders, setFolders] = useState<ApiFolderInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    // 添加共享文件夹弹窗状态（系统目录选择器不可用时的手动输入回退）
    const [addOpen, setAddOpen] = useState(false);
    const [addText, setAddText] = useState('');
    const [addPasswd, setAddPasswd] = useState('');
    const [adding, setAdding] = useState(false);
    // 系统目录选择器选中后，为所选文件夹设置访问密码（可选）的弹窗状态
    const [pwOpen, setPwOpen] = useState(false);
    const [pwText, setPwText] = useState('');
    const [pendingPath, setPendingPath] = useState<string | null>(null);
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

    /**
     * 添加共享文件夹：优先调用后端在本机弹出系统原生目录选择器，选中后询问是否设置访问密码；
     * 系统选择器不可用（如缺少 zenity/kdialog）时降级为手动输入弹窗（同样可设置密码）。
     */
    const handleAddClick = async () => {
        setAdding(true);
        try {
            const res = await pickDirectory();
            if (res.cancelled) return; // 用户取消选择
            const path = res.path;
            if (!path) throw new Error('未获取到所选目录路径');
            setPendingPath(path);
            setPwText('');
            setPwOpen(true); // 询问是否为该文件夹设置访问密码（可选，留空表示开放）
        } catch (e) {
            message.error(e instanceof Error ? e.message : '添加失败');
            setAddOpen(true); // 降级：手动输入路径
        } finally {
            setAdding(false);
        }
    };

    /** 系统目录选择器选中后确认添加（可为该文件夹设置访问密码） */
    const handleAddWithPassword = async () => {
        if (!pendingPath) return;
        setAdding(true);
        try {
            const passwd = pwText.trim();
            const added = await addFolders([pendingPath], passwd);
            message.success(`已添加共享文件夹「${added[0]?.name ?? pendingPath}」${passwd ? '（已设置访问密码）' : ''}`);
            setPwOpen(false);
            setPendingPath(null);
            setRefresh((r) => r + 1);
        } catch (e) {
            message.error(e instanceof Error ? e.message : '添加失败');
        } finally {
            setAdding(false);
        }
    };

    /** 手动输入多行路径并添加共享（系统目录选择器不可用时的备用方式，可为这些文件夹设置访问密码） */
    const handleAddManual = async () => {
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
            const passwd = addPasswd.trim();
            const added = await addFolders(paths, passwd);
            message.success(`已添加 ${added.length} 个共享文件夹${passwd ? '（均已设置访问密码）' : ''}`);
            setAddOpen(false);
            setAddText('');
            setAddPasswd('');
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
                    {record.auth && (
                        <Tooltip title="该文件夹设置了访问密码，其他节点需输入密码才能访问">
                            <LockOutlined className="text-amber-500 dark:text-amber-400"/>
                        </Tooltip>
                    )}
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
                        collapsible={false}
                        info={{
                            ip: node.ip,
                            os: node.os,
                            softwareVersion: node.softwareVersion,
                            uptime: node.uptime,
                        }}
                    />
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
                        <Button type="primary" icon={<FolderAddOutlined/>} loading={adding} onClick={handleAddClick}>
                            添加共享文件夹
                        </Button>
                    }
                    bodyStyle={{padding: 16}}
                >
                    {folders.length === 0 ? (
                        <Empty description="暂无共享文件夹，点击右上角「添加共享文件夹」选择文件夹开始共享"/>
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
        <>
            {content}

            {/* 系统目录选择器选中后：为所选文件夹设置访问密码（可选，留空表示开放） */}
            <Modal
                open={pwOpen}
                title="设置访问密码"
                okText="确认添加"
                cancelText="取消"
                confirmLoading={adding}
                onOk={handleAddWithPassword}
                onCancel={() => {
                    setPwOpen(false);
                    setPendingPath(null);
                }}
            >
                <div className="mb-1 text-sm text-neutral-600 dark:text-neutral-300">
                    将共享文件夹：<span className="font-medium">{pendingPath ?? ''}</span>
                </div>
                <div className="mb-2 text-xs text-neutral-500 dark:text-neutral-400">
                    设置后，其他节点需输入密码才能查看/下载该文件夹的内容（本机不受影响）。留空则不设置密码。
                </div>
                <Input.Password
                    value={pwText}
                    onChange={(e) => setPwText(e.target.value)}
                    placeholder="访问密码（可选）"
                    autoFocus
                />
            </Modal>

            {/* 手动输入路径弹窗：系统目录选择器不可用时的回退方式，每行一个目录绝对路径 */}
            <Modal
                open={addOpen}
                title="添加共享文件夹"
                okText="添加"
                cancelText="取消"
                confirmLoading={adding}
                onOk={handleAddManual}
                onCancel={() => {
                    setAddOpen(false);
                    setAddText('');
                    setAddPasswd('');
                }}
            >
                <div className="mb-2 text-sm text-neutral-500 dark:text-neutral-400">
                    系统目录选择器不可用，请手动输入要共享的目录绝对路径，每行一个：
                </div>
                <Input.TextArea
                    value={addText}
                    onChange={(e) => setAddText(e.target.value)}
                    placeholder={'例如：\n/home/user/Documents\n/media/data'}
                    autoSize={{minRows: 3, maxRows: 8}}
                />
                <div className="mb-1 mt-3 text-xs text-neutral-500 dark:text-neutral-400">
                    访问密码（可选，留空则不设置；多个路径使用同一密码）：
                </div>
                <Input.Password
                    value={addPasswd}
                    onChange={(e) => setAddPasswd(e.target.value)}
                    placeholder="访问密码（可选）"
                />
            </Modal>
        </>
    );
}
