'use client'

import React from 'react';
import {Alert, Empty, Spin} from 'antd';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {buildHosts} from '../_lib/nodes';
import {useNodesData} from '../_lib/use_nodes_data';

/**
 * 局域网节点面板：展示 mDNS 发现的节点（本机访问时排除本机，
 * 本机通过顶栏选项卡切到 /local 管理；远程访问时本机节点也作为局域网节点展示）。
 * 被 /nodes 局域网节点页使用。
 *
 * 数据获取与 peers 轮询由 useNodesData 统一提供（与总览页共用）。
 */
export default function NodesPanel() {
    const {node, nodeStatus, localFolders, peers, error} = useNodesData();

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
