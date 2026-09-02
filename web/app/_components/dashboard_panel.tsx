'use client'

import React, {useEffect, useState} from 'react';
import {Alert, Button, Spin, Statistic, Tag} from 'antd';
import {
  CloudServerOutlined,
  FolderOpenOutlined,
  HddOutlined,
  RocketOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import Link from 'next/link';
import {ProCard} from '@ant-design/pro-components';
import BrandMark from './brand_mark';
import {useTheme} from './app_theme';
import {useAccess} from './access_context';
import {formatSize} from '../_cards/folder_card';
import {excludeSelfPeers} from '../_lib/nodes';
import {
  fetchFolders,
  fetchPeers,
  type ApiFolderInfo,
  type ApiPeerInfo,
} from '../_lib/api';
import {PEER_REFRESH_INTERVAL} from '../_lib/constants';

/**
 * 主界面（/）：FileSpace 总览页。
 * 展示品牌（logo + 标题 + 软件版本）与全局统计：
 * 在线节点数（本机 + mDNS 发现的在线节点）、共享的总文件夹数、共享的总文件大小，
 * 并提供进入局域网节点 / 本机节点管理的快捷入口（远程访问时隐藏本机管理入口）。
 */

export default function DashboardPanel() {
    const {isDark, setMode} = useTheme();
    // 本机节点信息由 AccessProvider 统一提供（含访问来源 local 标记）
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
                setError(null);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '无法连接后端服务');
            }
        }
        // 定时刷新节点列表：节点上线/退出（含收到退出通知）后在线数即时更新，
        // 无需手动刷新页面。
        async function refreshPeers() {
            try {
                const ps = await fetchPeers();
                if (!cancelled) setPeers(ps);
            } catch {
                // 轮询失败保持现有数据，等待下一次
            }
        }
        load();
        const timer = setInterval(refreshPeers, PEER_REFRESH_INTERVAL);
        return () => {
            cancelled = true;
            clearInterval(timer);
        };
    }, [node]);

    // 统计口径：
    // - 在线节点数 = 本机（恒在线）+ mDNS 发现的在线节点（排除本机，防重复计数）。
    // - 共享文件夹数 / 总文件大小 = 本机共享 + 全部可见节点（含离线节点的缓存数据）。
    const remotePeers = node && peers ? excludeSelfPeers(peers, node.id) : [];
    const onlineNodes = 1 + remotePeers.filter((p) => p.online).length;
    const peerFolders = remotePeers.flatMap((p: ApiPeerInfo) => p.folders);
    const totalFolders = (localFolders?.length ?? 0) + peerFolders.length;
    const totalBytes =
        (localFolders?.reduce((sum, f) => sum + f.totalSize, 0) ?? 0) +
        peerFolders.reduce((sum, f) => sum + f.totalSize, 0);

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (nodeStatus === 'error') {
        // AccessProvider 拉取本机信息失败
        content = <Alert type="error" showIcon title="加载失败" description="无法连接后端服务"/>;
    } else if (nodeStatus === 'loading' || node === null || localFolders === null || peers === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else {
        content = (
            <div className="flex flex-col gap-4">
                {/* 品牌区：logo + 标题 + 软件版本（无边框） */}
                <div className="flex flex-col items-center gap-4 pb-2">
                    {/* 点击 logo 在当前亮/暗主题之间切换 */}
                    <div
                        className="flex h-28 w-28 cursor-pointer items-center justify-center rounded-3xl bg-blue-100 text-blue-600 transition-opacity hover:opacity-80 dark:bg-blue-400/20 dark:text-blue-400"
                        title="点击切换亮暗主题"
                        onClick={() => setMode(isDark ? 'light' : 'dark')}
                    >
                        <BrandMark className="h-20 w-20" label="文件空间 FileSpace"/>
                    </div>
                    <div className="text-center">
                        <div className="text-4xl font-bold tracking-wide text-neutral-900 dark:text-neutral-100">
                            文件空间
                        </div>
                        <div className="mt-1 text-base text-neutral-500 dark:text-neutral-400">
                            FileSpace · 局域网文件共享
                        </div>
                    </div>
                    <Tag icon={<RocketOutlined/>} color="blue">
                        v{node.softwareVersion}
                    </Tag>
                </div>

                {/* 全局统计 */}
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    <ProCard bordered bodyStyle={{padding: 16}}>
                        <Statistic
                            title={
                                <span className="flex items-center gap-1">
                                    <CloudServerOutlined className="text-blue-500 dark:text-blue-400"/>
                                    在线节点
                                </span>
                            }
                            value={onlineNodes}
                            suffix="个"
                        />
                        <div className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                            含本机在内，{remotePeers.filter((p) => p.online).length} 台远程节点在线
                        </div>
                    </ProCard>
                    <ProCard bordered bodyStyle={{padding: 16}}>
                        <Statistic
                            title={
                                <span className="flex items-center gap-1">
                                    <FolderOpenOutlined className="text-amber-500 dark:text-amber-400"/>
                                    共享文件夹
                                </span>
                            }
                            value={totalFolders}
                            suffix="个"
                        />
                        <div className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                            本机 {localFolders.length} 个 + 局域网 {peerFolders.length} 个
                        </div>
                    </ProCard>
                    <ProCard bordered bodyStyle={{padding: 16}}>
                        <Statistic
                            title={
                                <span className="flex items-center gap-1">
                                    <HddOutlined className="text-green-500 dark:text-green-400"/>
                                    总文件大小
                                </span>
                            }
                            value={formatSize(totalBytes)}
                        />
                        <div className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                            所有共享文件夹合计
                        </div>
                    </ProCard>
                </div>

                {/* 快捷入口（无边框） */}
                <div className="flex flex-col items-center gap-3 pt-8 sm:flex-row sm:justify-center">
                    <Link href="/nodes">
                        <Button type="primary" size="large" icon={<CloudServerOutlined/>}>
                            浏览局域网节点
                        </Button>
                    </Link>
                    {/* 远程访问时本机节点按局域网节点展示，本机共享管理仅限本机回环操作 */}
                    {node.local !== false && (
                        <Link href="/local">
                            <Button size="large" icon={<SettingOutlined/>}>
                                管理本机共享
                            </Button>
                        </Link>
                    )}
                </div>
            </div>
        );
    }

    // 全屏居中布局：垂直 + 水平居中，四周留白；内容限宽，避免统计卡片过宽
    return (
        <div className="flex min-h-screen flex-col items-center justify-center p-6 sm:p-10">
            <div className="w-full max-w-3xl">{content}</div>
        </div>
    );
}
