'use client'

import React, {useState} from 'react';
import {Alert, App as AntdApp, Empty, Input, Modal, Spin} from 'antd';
import HostCard, {type HostInfo} from '../_cards/host_card';
import {buildHosts, hostSyncAddress} from '../_lib/nodes';
import {useNodesData} from '../_lib/use_nodes_data';
import {addSync, pickDirectory, type ApiFolderInfo} from '../_lib/api';
import {errMsg} from '../_lib/errors';

/**
 * 局域网节点面板：展示 mDNS 发现的节点（本机访问时排除本机，
 * 本机通过顶栏选项卡切到 /local 管理；远程访问时本机节点也作为局域网节点展示）。
 * 被 /nodes 局域网节点页使用。
 *
 * 数据获取与 peers 轮询由 useNodesData 统一提供（与总览页共用）。
 *
 * 同步入口（仅本机访问时提供）：把某远程节点的共享文件夹镜像到本机。
 * pick-directory / sync/add 两个端点均仅允许本机回环调用，远程访问时隐藏同步按钮。
 */
export default function NodesPanel() {
    const {message} = AntdApp.useApp();
    const {node, nodeStatus, localFolders, peers, error} = useNodesData();

    // 同步流程状态：目标文件夹 + 已选本地路径（点完目录选择器后暂存，需密码时弹出确认框）
    const [syncTarget, setSyncTarget] = useState<{folder: ApiFolderInfo; host: HostInfo} | null>(null);
    const [syncPath, setSyncPath] = useState('');
    const [syncPasswd, setSyncPasswd] = useState('');
    const [syncSubmitting, setSyncSubmitting] = useState(false);

    // 由 node（AccessProvider）+ 本机共享 + 发现的节点派生主机列表
    const hosts: HostInfo[] | null =
        node !== null && localFolders !== null && peers !== null
            ? buildHosts(node, localFolders, peers)
            : null;

    // 仅本机回环访问时可创建同步任务（目录选择器与 sync/add 端点都只放行本机）
    const canSync = node?.local === true;

    /**
     * 点击「同步」：调用本机后端弹出系统目录选择器选本地目标目录；
     * 选中后若远程文件夹设置了访问密码（folder.auth），弹出密码确认框再提交，否则直接添加。
     */
    const handleSync = async (folder: ApiFolderInfo, host: HostInfo) => {
        try {
            const res = await pickDirectory();
            if (res.cancelled || !res.path) return; // 用户取消选择目录
            if (folder.auth) {
                // 需要密码：暂存目标与本地路径，弹出密码确认框
                setSyncTarget({folder, host});
                setSyncPath(res.path);
                setSyncPasswd('');
                return;
            }
            await submitSync({folder, host}, res.path, '');
        } catch (e) {
            message.error(errMsg(e, '打开目录选择器失败'));
        }
    };

    /** 提交同步任务：组装 Spec 调 POST /api/sync/add（远程定位取节点 listenAddr，缺失回退 ip） */
    const submitSync = async (
        target: {folder: ApiFolderInfo; host: HostInfo},
        local: string,
        passwd: string,
    ) => {
        setSyncSubmitting(true);
        try {
            const {host, port} = hostSyncAddress(target.host);
            await addSync({
                host,
                port,
                folder_id: target.folder.id,
                local,
                passwd: passwd || undefined,
            });
            message.success(`已添加同步任务：「${target.folder.name}」→ ${local}`);
            setSyncTarget(null);
            setSyncPath('');
            setSyncPasswd('');
        } catch (e) {
            message.error(errMsg(e, '添加同步任务失败'));
        } finally {
            setSyncSubmitting(false);
        }
    };

    /** 密码确认框的「确认」：用输入的密码提交同步任务 */
    const handleSyncConfirm = async () => {
        if (!syncTarget) return;
        const passwd = syncPasswd.trim();
        if (!passwd) {
            message.warning('请输入该文件夹的访问密码');
            return;
        }
        await submitSync(syncTarget, syncPath, passwd);
    };

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
        // min-w 取 500px：多节点时卡片保持较宽（节点信息/文件夹卡更舒展），
        // 单个节点时 flex-1 仍会撑满整行（比多节点卡片更宽）。
        content = (
            <div className="nodes-hscroll flex items-start gap-4 overflow-x-auto pb-2">
                {hosts.map((host) => (
                    <div key={host.id} className="min-w-[500px] flex-1">
                        <HostCard host={host} onSyncFolder={canSync ? handleSync : undefined}/>
                    </div>
                ))}
            </div>
        );
    }

    return (
        <>
            {content}

            {/* 同步密码确认框：远程文件夹设置了访问密码时，选完本地目录后弹出 */}
            <Modal
                open={syncTarget !== null}
                title="同步文件夹需要访问密码"
                okText="确认同步"
                cancelText="取消"
                confirmLoading={syncSubmitting}
                onOk={handleSyncConfirm}
                onCancel={() => {
                    setSyncTarget(null);
                    setSyncPath('');
                    setSyncPasswd('');
                }}
            >
                <div className="mb-1 text-sm text-neutral-600 dark:text-neutral-300">
                    将把远程文件夹 <span className="font-medium">{syncTarget?.folder.name}</span>
                    {' '}同步到本地：<span className="font-medium">{syncPath}</span>
                </div>
                <div className="mb-2 text-xs text-neutral-500 dark:text-neutral-400">
                    该文件夹设置了访问密码，输入密码后开始同步（本机后台持续单向镜像远程内容）。
                </div>
                <Input.Password
                    value={syncPasswd}
                    onChange={(e) => setSyncPasswd(e.target.value)}
                    onPressEnter={() => {
                        if (syncPasswd.trim()) handleSyncConfirm();
                    }}
                    placeholder="访问密码"
                    autoFocus
                />
            </Modal>
        </>
    );
}