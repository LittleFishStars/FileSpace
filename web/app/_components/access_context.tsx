'use client'

import React, {createContext, useContext, useEffect, useState} from 'react';
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
}

const AccessContext = createContext<AccessScope>({
    node: null,
    status: 'loading',
    isLocalAccess: null,
});

export function AccessProvider({children}: {children: React.ReactNode}) {
    const [node, setNode] = useState<ApiNodeInfo | null>(null);
    const [status, setStatus] = useState<AccessStatus>('loading');

    useEffect(() => {
        let cancelled = false;
        fetchNode()
            .then((n) => {
                if (cancelled) return;
                setNode(n);
                setStatus('ready');
            })
            .catch(() => {
                // 拉取失败：保持 node 为 null，由页面按 error 状态兜底展示
                if (!cancelled) setStatus('error');
            });
        return () => {
            cancelled = true;
        };
    }, []);

    const value: AccessScope = {
        node,
        status,
        isLocalAccess: node === null ? null : node.local,
    };

    return <AccessContext.Provider value={value}>{children}</AccessContext.Provider>;
}

/** 读取访问来源上下文（需在 AccessProvider 内使用） */
export function useAccess(): AccessScope {
    return useContext(AccessContext);
}
