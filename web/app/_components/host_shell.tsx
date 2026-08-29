'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Button, Empty, Spin} from 'antd';
import {FolderAddOutlined} from '@ant-design/icons';
import Link from 'next/link';
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
 * 主机列表页：仅展示 mDNS 发现的其他节点（本机节点不在此列出，
 * 本机共享文件夹在独立的「本机管理」页（/local）中管理）。
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
        <AppShell title="主机列表">
            {/* 本机管理入口：本机节点不占用主机列表卡片，单独进入管理页 */}
            <Link href="/local" className="mb-4 block">
                <Button
                    type="primary"
                    block
                    size="large"
                    icon={<FolderAddOutlined/>}
                    className="!h-14 !text-base"
                >
                    本机文件夹管理（添加 / 删除共享文件夹）
                </Button>
            </Link>
            {content}
        </AppShell>
    );
}
