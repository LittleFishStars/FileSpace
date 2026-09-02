'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Empty, Spin} from 'antd';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {useAccess} from './access_context';
import {buildHosts} from '../_lib/nodes';
import {fetchFolders, fetchPeers, type ApiFolderInfo, type ApiPeerInfo} from '../_lib/api';
import {PEER_REFRESH_INTERVAL} from '../_lib/constants';

/**
 * 局域网节点面板：展示 mDNS 发现的节点（本机访问时排除本机，
 * 本机通过顶栏选项卡切到 /local 管理；远程访问时本机节点也作为局域网节点展示）。
 * 被 /nodes 局域网节点页使用。
 *
 * 本机节点信息（含访问来源 local 标记）由 AccessProvider 统一提供；
 * 面板首次加载本机共享文件夹 + 节点列表，之后定时只刷新节点列表（peers）。
 */
export default function NodesPanel() {
    const {node, status: nodeStatus} = useAccess();
    const [localFolders, setLocalFolders] = useState<ApiFolderInfo[] | null>(null);
    const [peers, setPeers] = useState<ApiPeerInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        // 节点信息未就绪（加载中 / 拉取失败）时保持加载态，由 AccessProvider 状态兜底
        if (!node) return;
        let cancelled = false;
        async function load() {
            try {
                const [fs, ps] = await Promise.all([
                    fetchFolders(),
                    fetchPeers(),
                ]);
                if (cancelled) return;
                setLocalFolders(fs);
                setPeers(ps);
            } catch (e) {
                if (!cancelled) {
                    setError(e instanceof Error ? e.message : '无法连接后端服务');
                }
            }
        }
        // 定时刷新节点列表：节点上线/退出（含收到退出通知）后即时更新；
        // 本机共享文件夹基本不变，无需重复拉取。
        async function refresh() {
            try {
                const ps = await fetchPeers();
                if (!cancelled) setPeers(ps);
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
    }, [node]);

    // 由 node（AccessProvider）+ 本机共享 + 发现的节点派生主机列表
    const hosts: HostInfo[] | null =
        node !== null && localFolders !== null && peers !== null
            ? buildHosts(node, localFolders, peers)
            : null;

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (nodeStatus === 'error') {
        // AccessProvider 拉取本机信息失败
        content = <Alert type="error" showIcon title="加载失败" description="无法连接后端服务"/>;
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
