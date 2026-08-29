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
import {formatSize} from '../_cards/folder_card';
import {
  fetchFolders,
  fetchNode,
  fetchPeers,
  type ApiFolderInfo,
  type ApiNodeInfo,
  type ApiPeerInfo,
} from '../_lib/api';

/**
 * 主界面（/）：FileSpace 总览页。
 * 展示品牌（logo + 标题 + 软件版本）与全局统计：
 * 在线节点数（本机 + mDNS 发现的在线节点）、共享的总文件夹数、共享的总文件大小，
 * 并提供进入局域网节点 / 本机节点管理的快捷入口。
 */

/** 品牌标志：与 filespace-mark.svg 一致，stroke 用 currentColor 随主题（浅/深）自适应 */
function BrandMark({className}: {className?: string}) {
    return (
        <svg viewBox="74 85 390 383" className={className} role="img" aria-label="文件空间 FileSpace">
            <g stroke="currentColor" strokeWidth={14} strokeLinecap="round" fill="none">
                <path d="M214.6 192.3 L358.3 244.9"/>
                <path d="M181.1 225.6 L235.1 373.9"/>
                <path d="M366.9 284.7 L268.8 381.5"/>
                <circle cx="394" cy="258" r="38"/>
                <circle cx="246" cy="404" r="32"/>
                <circle cx="162" cy="173" r="56"/>
                <circle cx="162" cy="173" r="19" strokeWidth={9}/>
            </g>
        </svg>
    );
}

export default function DashboardPanel() {
    const [node, setNode] = useState<ApiNodeInfo | null>(null);
    const [localFolders, setLocalFolders] = useState<ApiFolderInfo[] | null>(null);
    const [peers, setPeers] = useState<ApiPeerInfo[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const [n, fs, ps] = await Promise.all([
                    fetchNode(),
                    fetchFolders(),
                    fetchPeers(),
                ]);
                if (cancelled) return;
                setNode(n);
                setLocalFolders(fs);
                setPeers(ps);
            } catch (e) {
                if (!cancelled) setError(e instanceof Error ? e.message : '无法连接后端服务');
            }
        }
        load();
        return () => {
            cancelled = true;
        };
    }, []);

    // 统计口径：
    // - 在线节点数 = 本机（恒在线）+ mDNS 发现的在线节点（排除本机，防重复计数）。
    // - 共享文件夹数 / 总文件大小 = 本机共享 + 全部可见节点（含离线节点的缓存数据）。
    const remotePeers = peers?.filter((p) => p.node.id !== node?.id) ?? [];
    const onlineNodes = 1 + remotePeers.filter((p) => p.online).length;
    const peerFolders = remotePeers.flatMap((p: ApiPeerInfo) => p.folders);
    const totalFolders = (localFolders?.length ?? 0) + peerFolders.length;
    const totalBytes =
        (localFolders?.reduce((sum, f) => sum + f.totalSize, 0) ?? 0) +
        peerFolders.reduce((sum, f) => sum + f.totalSize, 0);

    let content: React.ReactNode;
    if (error) {
        content = <Alert type="error" showIcon title="加载失败" description={error}/>;
    } else if (node === null || localFolders === null || peers === null) {
        content = (
            <div className="flex justify-center py-16">
                <Spin size="large"/>
            </div>
        );
    } else {
        content = (
            <div className="flex flex-col gap-4">
                {/* 品牌区：logo + 标题 + 软件版本 */}
                <ProCard bordered bodyStyle={{padding: '32px 16px'}}>
                    <div className="flex flex-col items-center gap-3">
                        <div className="flex h-20 w-20 items-center justify-center rounded-3xl bg-blue-100 text-blue-600 dark:bg-blue-400/20 dark:text-blue-400">
                            <BrandMark className="h-12 w-12"/>
                        </div>
                        <div className="text-center">
                            <div className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
                                文件空间
                            </div>
                            <div className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
                                FileSpace · 局域网文件共享
                            </div>
                        </div>
                        <Tag icon={<RocketOutlined/>} color="blue">
                            v{node.softwareVersion}
                        </Tag>
                    </div>
                </ProCard>

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

                {/* 快捷入口 */}
                <ProCard bordered bodyStyle={{padding: 16}}>
                    <div className="flex flex-col items-center gap-3 sm:flex-row sm:justify-center">
                        <Link href="/nodes">
                            <Button type="primary" size="large" icon={<CloudServerOutlined/>}>
                                浏览局域网节点
                            </Button>
                        </Link>
                        <Link href="/local">
                            <Button size="large" icon={<SettingOutlined/>}>
                                管理本机共享
                            </Button>
                        </Link>
                    </div>
                </ProCard>
            </div>
        );
    }

    return content;
}
