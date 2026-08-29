'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Empty, Spin} from 'antd';
import AppShell from './app_shell';
import HostCard, {type HostInfo} from '../_cards/host_card';
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
        auth: node.auth === true,
        folders: folders.map((f) => ({
            id: f.id,
            name: f.name,
            fileCount: f.fileCount,
            totalSize: f.totalSize,
            updatedAt: f.updatedAt,
            auth: f.auth === true,
        })),
    };
}

/**
 * 局域网节点页（/）：展示 mDNS 发现的其他节点（本机节点不在此列出，
 * 本机节点通过顶栏选项卡切到 /local 管理）。
 */
export default function HostShell() {
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

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (hosts === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else if (hosts.length === 0) {
        content = <Empty description="暂无其他节点"/>;
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
        <AppShell wide title="局域网节点">
            {content}
        </AppShell>
    );
}
