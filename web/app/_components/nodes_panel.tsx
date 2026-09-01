'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Empty, Spin} from 'antd';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {fetchFolders, fetchNode, fetchPeers, type ApiFolderInfo, type ApiNodeInfo, type ApiPeerInfo} from '../_lib/api';
import {PEER_REFRESH_INTERVAL} from '../_lib/constants';

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
 * 把发现的节点转换为主机列表。
 * - 本机访问（node.local === true）：排除本机（本机节点通过 /local 管理）。
 * - 远程访问（node.local === false）：本机节点也按普通局域网节点展示
 *   （本机共享文件夹通过 /api/folders 获取，访问走同源 API），不再单独管理。
 */
function buildHosts(node: ApiNodeInfo, localFolders: ApiFolderInfo[], peers: ApiPeerInfo[]): HostInfo[] {
    const list: HostInfo[] = [];
    if (node.local === false) {
        list.push(toHostInfo(node, localFolders));
    }
    for (const peer of peers) {
        if (peer.node && peer.node.id !== node.id) {
            list.push(toHostInfo(peer.node, peer.folders));
        }
    }
    return list;
}

/**
 * 局域网节点面板：展示 mDNS 发现的节点（本机访问时排除本机，
 * 本机通过顶栏选项卡切到 /local 管理；远程访问时本机节点也作为局域网节点展示）。
 * 被 /nodes 局域网节点页使用。
 */
export default function NodesPanel() {
    const [hosts, setHosts] = useState<HostInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [node, localFolders, peers] = await Promise.all([
                    fetchNode(),
                    fetchFolders(),
                    fetchPeers(),
                ]);
                if (cancelled) return;
                setHosts(buildHosts(node, localFolders, peers));
            } catch (e) {
                if (!cancelled) {
                    setError(e instanceof Error ? e.message : '无法连接后端服务');
                }
            }
        }
        // 定时刷新节点列表：节点上线/退出（含收到退出通知）后即时更新，无需手动刷新页面。
        async function refresh() {
            try {
                const [node, localFolders, peers] = await Promise.all([
                    fetchNode(),
                    fetchFolders(),
                    fetchPeers(),
                ]);
                if (!cancelled) setHosts(buildHosts(node, localFolders, peers));
            } catch {
                // 轮询失败保持现有数据，等待下一次
            }
        }
        load();
        const timer = setInterval(refresh, PEER_REFRESH_INTERVAL);
        return () => {
            cancelled = true;
            clearInterval(timer);
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

    return content;
}
