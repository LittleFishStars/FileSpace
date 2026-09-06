'use client'

import React, {createContext, useCallback, useContext, useEffect, useState} from 'react';
import {fetchNode, type ApiNodeInfo} from '../_lib/api';

/**
 * 访问来源上下文：在应用顶层统一拉取一次 /api/node，
 * 供 AppShell（菜单）、Dashboard / Nodes / Local 面板与文件夹页共享，
 * 避免每个组件各自重复请求节点信息。
 *
 * 关键字段 node.local：后端按请求来源（是否回环）填充——
 * 本机访问 local=true，远程设备访问 local=false（本机节点按局域网节点展示）。
 */

/** 节点信息加载状态 */
export type AccessStatus = 'loading' | 'ready' | 'error';

export interface AccessScope {
    /** 当前访问的本机节点信息；status 非 ready 时为 null */
    node: ApiNodeInfo | null;
    /** 节点信息加载状态 */
    status: AccessStatus;
    /** 是否本机（回环）访问；status 非 ready 时为 null */
    isLocalAccess: boolean | null;
    /** 重新拉取本机节点信息（修改主机名等操作后调用，保持各页面展示同步） */
    refresh: () => void;
}

const AccessContext = createContext<AccessScope>({
    node: null,
    status: 'loading',
    isLocalAccess: null,
    // 初始为 no-op；Provider 挂载后替换为真实拉取（见下方 refresh）
    refresh: () => {},
});

export function AccessProvider({children}: {children: React.ReactNode}) {
    const [node, setNode] = useState<ApiNodeInfo | null>(null);
    const [status, setStatus] = useState<AccessStatus>('loading');

    // 拉取节点信息：初次挂载与 refresh 触发时执行（refresh 不做 loading 回落，
    // 复用 setNode 原子更新，避免修改主机名后页面闪烁回加载态）
    const refresh = useCallback(() => {
        fetchNode()
            .then((n) => {
                setNode(n);
                setStatus('ready');
            })
            .catch(() => {
                // 拉取失败：保留现有数据；首次失败（node 仍为 null）时由页面按 error 状态兜底
                setStatus('error');
            });
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const value: AccessScope = {
        node,
        status,
        isLocalAccess: node === null ? null : node.local,
        refresh,
    };

    return <AccessContext.Provider value={value}>{children}</AccessContext.Provider>;
}

/** 读取访问来源上下文（需在 AccessProvider 内使用） */
export function useAccess(): AccessScope {
    return useContext(AccessContext);
}
