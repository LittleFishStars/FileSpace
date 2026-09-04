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
