// 节点/主机列表共享工具：把后端数据转换为前端展示结构，
// 供局域网节点面板（nodes_panel）与总览页（dashboard_panel）复用，
// 保证「本机节点按局域网节点展示」的口径一致。

import type {ApiFolderInfo, ApiNodeInfo, ApiPeerInfo} from './api';
import type {HostInfo} from '../_cards/host_card';

/** 把后端 NodeInfo + folders 转换为前端 HostInfo（folders 直接透传后端模型） */
function toHostInfo(node: ApiNodeInfo, folders: ApiFolderInfo[]): HostInfo {
    return {
        id: node.id,
        hostname: node.hostname,
        ip: node.ip,
        os: node.os,
        status: node.status === 'online' ? 'online' : 'offline',
        uptime: node.uptime,
        softwareVersion: node.softwareVersion,
        auth: node.auth,
        /** 节点 API 地址 host:port（同步/浏览远端文件时使用；缺失时后端回退默认端口） */
        listenAddr: node.listenAddr,
        folders,
    };
}

/**
 * 构建主机列表。
 * mDNS 缓存已排除本机节点（后端 handleEntry 对自身 ID 直接跳过），
 * 故 peers 中不包含本机；本机节点是否展示由访问来源决定：
 * - 远程访问（node.local === false）：本机节点也按普通局域网节点展示
 *   （本机共享文件夹经 /api/folders 获取，访问走同源 API）。
 * - 本机访问（node.local === true）：不展示本机（通过 /local 管理）。
 */
export function buildHosts(node: ApiNodeInfo, localFolders: ApiFolderInfo[], peers: ApiPeerInfo[]): HostInfo[] {
    const list: HostInfo[] = [];
    if (!node.local) {
        list.push(toHostInfo(node, localFolders));
    }
    for (const peer of peers) {
        list.push(toHostInfo(peer.node, peer.folders));
    }
    return list;
}

/**
 * 从节点信息解析同步用的远程定位（host + port）：
 * 优先取 listenAddr（host:port），缺失时回退 ip + 默认端口（port=0 交由后端补 8080）。
 * 与后端 sync.Spec / discovery.peerAddr 的口径一致。
 * 入参为结构兼容类型：ApiNodeInfo 与 HostInfo 均满足（只读 ip / listenAddr 两个字段）。
 */
export function hostSyncAddress(node: {ip: string; listenAddr?: string}): {host: string; port: number} {
    const addr = node.listenAddr?.trim() ?? '';
    const idx = addr.lastIndexOf(':');
    if (idx > 0) {
        const port = Number(addr.slice(idx + 1));
        if (Number.isInteger(port) && port > 0 && port <= 65535) {
            return {host: addr.slice(0, idx), port};
        }
    }
    return {host: node.ip, port: 0}; // 0 表示使用默认端口 8080
}
