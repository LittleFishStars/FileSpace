'use client';

// 「节点 + 本机共享文件夹 + mDNS 发现的 peers」这一组合的获取与轮询逻辑，
// 是总览页（dashboard_panel）与局域网节点页（nodes_panel）的共同需求。
// 抽成 hook 以避免两处各写一遍：首次并发拉本机共享 + peers，
// 之后仅定时（PEER_REFRESH_INTERVAL）与页面切回前台时刷新 peers（本机共享基本不变）。

import {useEffect, useState} from 'react';
import {useAccess, type AccessStatus} from '../_components/access_context';
import {
    fetchFolders,
    fetchPeers,
    type ApiFolderInfo,
    type ApiNodeInfo,
    type ApiPeerInfo,
} from './api';
import {PEER_REFRESH_INTERVAL} from './constants';
import {errMsg} from './errors';

export interface NodesData {
    /** 本机节点信息（AccessProvider 提供），未就绪时为 null */
    node: ApiNodeInfo | null;
    /** AccessProvider 拉取 /api/node 的状态：页面据此渲染加载 / 失败兜底 */
    nodeStatus: AccessStatus;
    /** 本机共享文件夹列表，未就绪时为 null */
    localFolders: ApiFolderInfo[] | null;
    /** mDNS 发现的其他节点（含其共享文件夹），未就绪时为 null */
    peers: ApiPeerInfo[] | null;
    /** 数据加载错误（成功恢复时自动清空） */
    error: string | null;
}

export function useNodesData(): NodesData {
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
                const [fs, ps] = await Promise.all([fetchFolders(), fetchPeers()]);
                if (cancelled) return;
                setLocalFolders(fs);
                setPeers(ps);
                setError(null);
            } catch (e) {
                if (!cancelled) setError(errMsg(e, '无法连接后端服务'));
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
        // 页面从后台切回前台时立即刷新：浏览器会对后台标签页的定时器节流，
        // 切回时可能带着过期数据，补拉一次让在线状态即时更新。
        function onVisible() {
            if (document.visibilityState === 'visible') refresh();
        }
        load();
        const timer = setInterval(refresh, PEER_REFRESH_INTERVAL);
        document.addEventListener('visibilitychange', onVisible);
        return () => {
            cancelled = true;
            clearInterval(timer);
            document.removeEventListener('visibilitychange', onVisible);
        };
    }, [node]);

    return {node, nodeStatus, localFolders, peers, error};
}
