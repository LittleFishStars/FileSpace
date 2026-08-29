'use client'

import React, {createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {App as AntdApp, ConfigProvider, theme as antdTheme} from 'antd';
import zhCN from 'antd/locale/zh_CN';

/**
 * 主题模式：
 * - system：跟随浏览器 prefers-color-scheme
 * - light / dark：手动覆盖（持久化到 localStorage）
 */
export type ThemeMode = 'system' | 'light' | 'dark';

interface ThemeContextValue {
    mode: ThemeMode;
    isDark: boolean;
    setMode: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const DARK_QUERY = '(prefers-color-scheme: dark)';
const STORAGE_KEY = 'file-space-theme-mode';

function subscribe(onStoreChange: () => void): () => void {
    const mq = window.matchMedia(DARK_QUERY);
    mq.addEventListener('change', onStoreChange);
    return () => mq.removeEventListener('change', onStoreChange);
}

function getSnapshot(): boolean {
    return window.matchMedia(DARK_QUERY).matches;
}

// 服务端渲染时 window 不可用，回退为浅色；
// 水合后 useSyncExternalStore 会用 getSnapshot 重新校正，从而与浏览器配置保持一致。
function getServerSnapshot(): boolean {
    return false;
}

function readStoredMode(): ThemeMode {
    if (typeof window === 'undefined') return 'system';
    const value = window.localStorage.getItem(STORAGE_KEY);
    return value === 'light' || value === 'dark' || value === 'system' ? value : 'system';
}

export default function AppTheme({children}: { children: React.ReactNode }) {
    const systemDark = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
    // 持久化偏好用惰性初始值读取（服务端回退为 system），避免在 effect 中同步 setState
    const [mode, setModeState] = useState<ThemeMode>(readStoredMode);

    const isDark = mode === 'dark' || (mode === 'system' && systemDark);

    const setMode = useCallback((m: ThemeMode) => {
        setModeState(m);
        try {
            window.localStorage.setItem(STORAGE_KEY, m);
        } catch {
            // localStorage 不可用时忽略（不影响本次会话内生效）
        }
    }, []);

    // 在 <html> 上开关 .dark，让 Tailwind dark: 变体与页面背景跟随当前主题
    useEffect(() => {
        const root = document.documentElement;
        if (isDark) root.classList.add('dark');
        else root.classList.remove('dark');
    }, [isDark]);

    const value = useMemo<ThemeContextValue>(
        () => ({mode, isDark, setMode}),
        [mode, isDark, setMode],
    );

    return (
        <ThemeContext.Provider value={value}>
            <ConfigProvider
                locale={zhCN}
                theme={{
                    algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
                    token: {
                        colorPrimary: '#4f7cd1',
                        borderRadius: 8,
                    },
                }}
            >
                {/* antd App 上下文：提供 message / notification / modal 的 hook API（App.useApp） */}
                <AntdApp>{children}</AntdApp>
            </ConfigProvider>
        </ThemeContext.Provider>
    );
}

export function useTheme(): ThemeContextValue {
    const ctx = useContext(ThemeContext);
    if (!ctx) {
        throw new Error('useTheme 必须在 <AppTheme> 内部使用');
    }
    return ctx;
}
