'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Empty, Spin} from 'antd';
import AppShell from './app_shell';
import HostCard, {type HostInfo} from '../_cards/host_card';
import LocalPanel from './local_panel';
import {fetchNode, fetchPeers, type ApiFolderInfo, type ApiNodeInfo} from '../_lib/api';

/** 把后端 NodeInfo + folders 转换为前端 HostInfo */
function toHostInfo(node: ApiNodeInfo, folders: ApiFolderInfo[]): HostInfo {
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
 * 主页：顶栏两个选项卡（标题与主题切换按钮所在的那一行）。
 * - 局域网节点：mDNS 发现的其他节点（本机节点不在此列出）
 * - 本机节点：本机节点信息 + 共享文件夹管理（添加 / 删除），与 /local 页内容一致
 */
export default function HostShell() {
    // 当前选项卡：lan=局域网节点（默认），local=本机节点
    const [tab, setTab] = useState<'lan' | 'local'>('lan');
    const [hosts, setHosts] = useState<HostInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [node, peers] = await Promise.all([
                    fetchNode(),
                    fetchPeers(),
                ]);
                if (cancelled) return;
                const list: HostInfo[] = [];
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

    /** 局域网节点视图内容 */
    let lanContent: React.ReactNode;
    if (error) {
        lanContent = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (hosts === null) {
        lanContent = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else if (hosts.length === 0) {
        lanContent = <Empty description="暂无其他节点"/>;
    } else {
        lanContent = (
            <div className="flex flex-col gap-4">
                {hosts.map((host) => (
                    <HostCard key={host.id} host={host}/>
                ))}
            </div>
        );
    }

    return (
        <AppShell
            wide
            title="主机列表"
            topTabs={{
                items: [
                    {key: 'lan', label: '局域网节点'},
                    {key: 'local', label: '本机节点'},
                ],
                activeKey: tab,
                onChange: (key) => setTab(key as 'lan' | 'local'),
            }}
        >
            {tab === 'lan' ? lanContent : <LocalPanel/>}
        </AppShell>
    );
}
