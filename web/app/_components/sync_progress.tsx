'use client'

import React, {useEffect, useState} from 'react';
import {Progress} from 'antd';
import {useAccess} from './access_context';
import {fetchSyncStatus, type ApiSyncTask} from '../_lib/api';

/**
 * 右下角浮动的同步进度条：有后台同步任务运行时，用圆形进度环展示当前进度。
 *
 *  - 圆形中间显示下载速度（下载阶段）；获取文件列表阶段无速度，显示阶段名。
 *  - 环的颜色随阶段切换：获取文件列表（listing）为蓝色，下载（downloading）为绿色，
 *    直观区分两个阶段。
 *  - 仅本机访问时轮询 /api/sync/status（该接口只放行回环调用）；无活跃任务时隐藏。
 */
export default function SyncProgress() {
    const {isLocalAccess} = useAccess();

    // 当前活跃同步任务（取第一个处于获取列表/下载阶段的任务；无则 null 隐藏）
    const [task, setTask] = useState<ApiSyncTask | null>(null);

    useEffect(() => {
        if (isLocalAccess !== true) return; // 远程访问：状态接口不放行，不轮询
        let cancelled = false;

        const poll = async () => {
            try {
                const tasks = await fetchSyncStatus();
                if (cancelled) return;
                const active = tasks.find(
                    (t) => t.phase === 'waiting' || t.phase === 'listing' || t.phase === 'downloading',
                );
                // 有活跃任务就显示；全部空闲/结束则隐藏（对账周期间隙不残留旧进度）
                setTask(active ?? null);
            } catch {
                // 后端暂不可达/非本机：静默，保持当前显示
            }
        };
        poll();
        const id = window.setInterval(poll, 1000);
        return () => {
            cancelled = true;
            window.clearInterval(id);
        };
    }, [isLocalAccess]);

    if (task === null) return null;

    // 阶段驱动配色与中心内容：waiting=灰色（等待首次对账），listing=蓝（获取列表），
    // downloading=绿（下载）
    const waiting = task.phase === 'waiting';
    const listing = task.phase === 'listing';
    const color = waiting ? '#9ca3af' : listing ? '#1677ff' : '#52c41a';
    const percent =
        task.total > 0 ? Math.min(100, Math.round((task.done / task.total) * 100)) : 0;

    return (
        <div
            className="fixed bottom-6 right-6 z-50 select-none"
            title={`同步 ${task.remote} → ${task.local}`}
        >
            <div
                className="flex items-center gap-3 rounded-2xl border border-gray-200/70 bg-white/95 px-3 py-2 shadow-lg backdrop-blur dark:border-white/10 dark:bg-gray-900/90"
            >
                <Progress
                    type="circle"
                    size={72}
                    percent={percent}
                    strokeColor={color}
                    strokeWidth={8}
                    format={() => (
                        <div className="text-center leading-tight">
                            <div
                                className="text-sm font-semibold"
                                style={{color}}
                            >
                                {waiting
                                    ? '等待中'
                                    : listing
                                      ? '获取列表'
                                      : formatSpeed(task.speed)}
                            </div>
                            <div className="text-xs text-gray-400 dark:text-gray-500">
                                {formatSize(task.done)}
                            </div>
                        </div>
                    )}
                />
                <div className="max-w-40 leading-snug">
                    <div className="text-sm font-medium text-gray-700 dark:text-gray-200">
                        {waiting
                            ? '正在等待首次对账…'
                            : listing
                              ? '正在获取远端文件列表…'
                              : '正在同步…'}
                    </div>
                    <div className="mt-0.5 truncate text-xs text-gray-400 dark:text-gray-500">
                        {task.remote}
                    </div>
                </div>
            </div>
        </div>
    );
}

/** 把字节/秒格式化为可读速度（如 2.3 MB/s） */
function formatSpeed(bytesPerSec: number): string {
    if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '0 B/s';
    const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
    let v = bytesPerSec;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

/** 把字节数格式化为可读大小（复用展示层一致的 1 位小数规则） */
function formatSize(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = bytes;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}