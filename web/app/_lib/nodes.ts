// 节点/主机列表共享工具：把后端数据转换为前端展示结构，
// 供局域网节点面板（nodes_panel）与总览页（dashboard_panel）复用，
// 保证「排除本机」「本机节点按局域网节点展示」的口径一致。

import type {ApiFolderInfo, ApiNodeInfo, ApiPeerInfo} from './api';
import type {HostInfo} from '../_cards/host_card';

/** 把后端 NodeInfo + folders 转换为前端 HostInfo */
export function toHostInfo(node: ApiNodeInfo, folders: ApiFolderInfo[]): HostInfo {
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

/** 排除本机节点（按节点 id，mDNS 缓存后端已排除本机，此处为双保险） */
export function excludeSelfPeers(peers: ApiPeerInfo[], selfId: string): ApiPeerInfo[] {
    return peers.filter((p) => p.node && p.node.id !== selfId);
}

/**
 * 构建主机列表。
 * - 本机访问（node.local === true）：排除本机（本机节点通过 /local 管理）。
 * - 远程访问（node.local === false）：本机节点也按普通局域网节点展示
 *   （本机共享文件夹通过 /api/folders 获取，访问走同源 API），不再单独管理。
 */
export function buildHosts(node: ApiNodeInfo, localFolders: ApiFolderInfo[], peers: ApiPeerInfo[]): HostInfo[] {
    const list: HostInfo[] = [];
    if (node.local === false) {
        list.push(toHostInfo(node, localFolders));
    }
    for (const peer of excludeSelfPeers(peers, node.id)) {
        list.push(toHostInfo(peer.node, peer.folders));
    }
    return list;
}
