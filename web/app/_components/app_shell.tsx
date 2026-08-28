'use client'

import React, {useSyncExternalStore} from 'react';
import {Segmented} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import {PageContainer, ProLayout} from '@ant-design/pro-components';
import {useTheme, type ThemeMode} from './app_theme';

/**
 * 客户端挂载检测：服务端水合前为 false，客户端挂载后为 true。
 * 用它在客户端才渲染 ProLayout，避免服务端渲染依赖 window.matchMedia 的
 * 响应式布局（screen-md 等）导致 hydration mismatch。
 */
function subscribeNoop() {
    return () => {};
}

function getClientSnapshot() {
    return true;
}

function getServerSnapshot() {
    return false;
}

/**
 * 应用外壳：ProLayout 顶部导航 + 右上角主题切换 + 页面容器。
 * 各页面共用，传入标题与内容即可。
 */
export default function AppShell({
    title,
    wide = false,
    children,
}: {
    title: string;
    /** 内容区是否放宽（文件浏览等表格页用），默认窄栏 */
    wide?: boolean;
    children: React.ReactNode;
}) {
    const mounted = useSyncExternalStore(subscribeNoop, getClientSnapshot, getServerSnapshot);
    const {mode, setMode} = useTheme();

    // 挂载前渲染占位，避免服务端渲染 ProLayout。
    if (!mounted) {
        return <div className="min-h-screen"/>;
    }

    const themeOptions = [
        {label: '系统', value: 'system' as ThemeMode, icon: <DesktopOutlined/>},
        {label: '浅色', value: 'light' as ThemeMode, icon: <SunOutlined/>},
        {label: '深色', value: 'dark' as ThemeMode, icon: <MoonOutlined/>},
    ];

    return (
        <ProLayout
            title={'文件空间'}
            logo={undefined}
            layout={'top'}
            actionsRender={() => [
                <Segmented
                    key="theme-toggle"
                    size="middle"
                    style={{marginRight: 16}}
                    value={mode}
                    onChange={(value) => setMode(value as ThemeMode)}
                    options={themeOptions}
                />,
            ]}
        >
            <PageContainer
                header={{
                    title,
                }}
            >
                <div className={`mx-auto w-full ${wide ? 'max-w-5xl' : 'max-w-2xl'}`}>
                    {children}
                </div>
            </PageContainer>
        </ProLayout>
    );
}
