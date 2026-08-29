'use client'

import React, {createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useState, useSyncExternalStore} from 'react';
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

/**
 * 提取样式文本的「选择器签名」：所有规则选择器（跳过 @ 规则）去重排序后拼接。
 * 同一组件在亮/暗主题下的样式节点选择器相同、仅属性值不同 → 签名相同。
 */
function selectorSignature(text: string): string {
    const sels = new Set<string>();
    const re = /([^{}]+)\{/g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
        const sel = m[1].trim();
        if (!sel || sel.startsWith('@')) continue;
        sels.add(sel);
    }
    return [...sels].sort().join('|');
}

/**
 * 清理同组件的旧主题样式节点。
 *
 * 背景：cssinjs 主题切换时，新主题的组件样式以新的 styleId 追加到 <head> 队尾，
 * 旧主题节点不删除、位置不变（非 cssVar 模式的 pro-components 样式为硬编码 token 值，
 * 每次切换都会生成新节点）。切回旧主题时 updateCSS 只更新已存在节点的内容、不重排位置，
 * 导致后插入的节点永远覆盖先插入的节点（如亮色卡片样式持续覆盖暗色）。
 *
 * 这里在主题切换后按「选择器签名」分组，每组保留最后一个（当前主题）节点，
 * 删除前面的旧主题重复节点。CSS 变量节点（含 --ant-）与纯 @keyframes 节点被排除。
 */
function pruneDuplicateStyles() {
    const nodes = Array.from(document.querySelectorAll<HTMLStyleElement>('style[data-rc-order="prependQueue"]'));
    const groups = new Map<string, HTMLStyleElement[]>();
    for (const node of nodes) {
        const text = node.textContent ?? '';
        // 跳过 CSS 变量节点（自定义属性 --ant-*）与无选择器节点（keyframes 等）
        if (text.includes('--ant-')) continue;
        const sig = selectorSignature(text);
        if (!sig) continue;
        const list = groups.get(sig);
        if (list) list.push(node);
        else groups.set(sig, [node]);
    }
    for (const list of groups.values()) {
        if (list.length > 1) {
            for (const node of list.slice(0, -1)) node.remove();
        }
    }
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

    // 主题切换后（新样式已由 cssinjs 在 useInsertionEffect 中插入），
    // 在绘制前清理旧主题的重复组件样式节点，避免后插入的旧主题样式持续覆盖当前主题
    useLayoutEffect(() => {
        if (typeof window === 'undefined') return;
        pruneDuplicateStyles();
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
                    // cssVar 模式：token 输出为 :root 上的 CSS 变量（--ant-*），组件样式引用变量。
                    // 主题切换时仅更新同一变量节点的值，组件样式不重新生成，
                    // 避免 cssinjs 样式节点按插入顺序排队导致旧主题样式被后插入的新主题样式覆盖
                    // （症状：暗→亮正常，亮→暗时 antd 组件背景仍保持亮色）。
                    cssVar: {},
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
