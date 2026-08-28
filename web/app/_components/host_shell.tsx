'use client'

import React, {useEffect, useState, useSyncExternalStore} from 'react';
import {Alert, Empty, Segmented, Spin} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import {PageContainer, ProLayout} from '@ant-design/pro-components';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {useTheme, type ThemeMode} from './app_theme';

/**
 * 客户端挂载检测：服务端水合前为 false，客户端挂载后为 true。
 * 用它在客户端才渲染 ProLayout，避免服务端渲染依赖 window.matchMedia 的
 * 响应式布局（screen-md 等）导致 hydration mismatch。
 */
function subscribeNoop() {
    return () => {};
}

function getClientSnapshot() {
    return true;
}

function getServerSnapshot() {
    return false;
}

// 后端 API 数据类型（与 backend/internal/model 对齐）
interface NodeInfo {
    id: string;
    hostname: string;
    ip: string;
    os: string;
    softwareVersion: string;
    status: string;
    uptime: string;
    listenAddr: string;
}

interface FolderInfo {
    id: string;
    name: string;
    path: string;
    fileCount: number;
    totalSize: number;
    updatedAt: string;
}

interface PeerInfo {
    node: NodeInfo;
    folders: FolderInfo[];
    online: boolean;
    lastSeen: string;
}

async function fetchJSON<T>(url: string): Promise<T> {
    const res = await fetch(url);
    if (!res.ok) {
        throw new Error(`${url} 返回 ${res.status}`);
    }
    return res.json() as Promise<T>;
}

/** 把后端 NodeInfo + folders 转换为前端 HostInfo */
function toHostInfo(node: NodeInfo, folders: FolderInfo[]): HostInfo {
    return {
        id: node.id,
        hostname: node.hostname,
        ip: node.ip,
        os: node.os,
        status: node.status === 'online' ? 'online' : 'offline',
        uptime: node.uptime,
        softwareVersion: node.softwareVersion,
        folders: folders.map((f) => ({
            id: f.id,
            name: f.name,
            fileCount: f.fileCount,
            totalSize: f.totalSize,
            updatedAt: f.updatedAt,
        })),
    };
}

/**
 * 应用外壳。
 * 负责页面顶层布局（ProLayout 顶部导航 + PageContainer），右上角提供主题切换控件，
 * 并加载后端数据：本节点 + mDNS 发现的其他节点（含各自共享文件夹）。
 */
export default function HostShell() {
    const mounted = useSyncExternalStore(subscribeNoop, getClientSnapshot, getServerSnapshot);
    const {mode, setMode} = useTheme();
    const [hosts, setHosts] = useState<HostInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [node, folders, peers] = await Promise.all([
                    fetchJSON<NodeInfo>('/api/node'),
                    fetchJSON<FolderInfo[]>('/api/folders'),
                    fetchJSON<PeerInfo[]>('/api/peers'),
                ]);
                if (cancelled) return;
                const list: HostInfo[] = [toHostInfo(node, folders)];
                for (const peer of peers) {
                    if (peer.node && peer.node.id !== node.id) {
                        list.push(toHostInfo(peer.node, peer.folders));
                    }
                }
                setHosts(list);
            } catch (e) {
                if (!cancelled) {
                    setError(e instanceof Error ? e.message : '无法连接后端服务');
                }
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    // 挂载前渲染占位，避免服务端渲染 ProLayout。
    if (!mounted) {
        return <div className="min-h-screen"/>;
    }

    const themeOptions = [
        {label: '系统', value: 'system' as ThemeMode, icon: <DesktopOutlined/>},
        {label: '浅色', value: 'light' as ThemeMode, icon: <SunOutlined/>},
        {label: '深色', value: 'dark' as ThemeMode, icon: <MoonOutlined/>},
    ];

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon message="加载失败" description={error}/>;
    } else if (hosts === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else if (hosts.length === 0) {
        content = <Empty description="暂无节点"/>;
    } else {
        content = (
            <div className="flex flex-col gap-4">
                {hosts.map((host) => (
                    <HostCard key={host.id} host={host}/>
                ))}
            </div>
        );
    }

    return (
        <ProLayout
            title={'文件空间'}
            logo={undefined}
            layout={'top'}
            actionsRender={() => [
                <Segmented
                    key="theme-toggle"
                    size="middle"
                    style={{marginRight: 16}}
                    value={mode}
                    onChange={(value) => setMode(value as ThemeMode)}
                    options={themeOptions}
                />,
            ]}
        >
            <PageContainer
                header={{
                    title: '主机列表',
                }}
            >
                <div className="mx-auto max-w-2xl">
                    {content}
                </div>
            </PageContainer>
        </ProLayout>
    );
}
