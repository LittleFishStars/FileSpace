'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Empty, Spin} from 'antd';
import AppShell from './app_shell';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {fetchNode, fetchPeers, fetchFolders, type ApiFolderInfo, type ApiNodeInfo} from '../_lib/api';

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

/** 主机列表页：本节点 + mDNS 发现的其他节点（含各自共享文件夹） */
export default function HostShell() {
    const [hosts, setHosts] = useState<HostInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [node, folders, peers] = await Promise.all([
                    fetchNode(),
                    fetchFolders(),
                    fetchPeers(),
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

    return <AppShell title="主机列表">{content}</AppShell>;
}
