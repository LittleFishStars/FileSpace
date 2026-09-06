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
        // 节点卡片横向滚动：flex-1 让卡片优先收缩宽度尽量全部显示，
        // 缩到 min-w 最小宽度仍放不下时出现横向滚动条。
        content = (
            <div className="nodes-hscroll flex items-start gap-4 overflow-x-auto pb-2">
                {hosts.map((host) => (
                    <div key={host.id} className="min-w-[300px] flex-1">
                        <HostCard host={host}/>
                    </div>
                ))}
            </div>
        );
    }

    return content;
}
