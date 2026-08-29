'use client'

import AppShell from '../_components/app_shell';
import LocalPanel from '../_components/local_panel';

/**
 * 本机管理页（/local）。
 * 与主页「本机节点」选项卡共用 LocalPanel（本机节点信息 + 共享文件夹管理）。
 * 保留独立路由，供文件夹浏览页（/folders）的面包屑返回使用。
 */
export default function LocalPage() {
    return (
        <AppShell wide title="本机管理">
            <LocalPanel/>
        </AppShell>
    );
}
