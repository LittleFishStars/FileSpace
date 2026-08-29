'use client'

import AppShell from '../_components/app_shell';
import NodesPanel from '../_components/nodes_panel';

/**
 * 局域网节点页（/nodes）。
 * 与顶栏「局域网节点」选项卡对应：展示 mDNS 发现的其他节点及其共享文件夹。
 */
export default function NodesPage() {
    return (
        <AppShell wide title="局域网节点">
            <NodesPanel/>
        </AppShell>
    );
}
