'use client'

import React, {useSyncExternalStore} from 'react';
import {Segmented} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import {PageContainer, ProLayout} from '@ant-design/pro-components';
import HostCard from '../_cards/host_card';
import {useTheme, type ThemeMode} from './app_theme';

/**
 * 客户端挂载检测：服务端水合前为 false，客户端挂载后为 true。
 * 用它在客户端才渲染 ProLayout，避免服务端渲染依赖 window.matchMedia 的
 * 响应式布局（screen-md 等）导致 hydration mismatch。
 * 相比 useEffect + setState 的门控，这里用 useSyncExternalStore 更符合 React 惯例。
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
 * 应用外壳。
 * 负责页面顶层布局（ProLayout 顶部导航 + PageContainer），并在右上角提供主题切换控件。
 *
 * 主题控件使用 antd Segmented 实现「跟随系统 / 浅色 / 深色」三挡切换，
 * 右侧留出一点边距，避免贴边。
 *
 * ProLayout 依赖 window.matchMedia 计算响应式 class（如 screen-md），
 * 服务端渲染时 window 不可用，导致服务端/客户端 HTML 属性不一致，
 * 从而触发 hydration mismatch。因此本组件在挂载完成前不渲染 ProLayout，
 * 待 useEffect 后将 mounted 置为 true 再渲染，确保仅客户端构建该子树。
 */
export default function HostShell() {
    const mounted = useSyncExternalStore(subscribeNoop, getClientSnapshot, getServerSnapshot);
    const {mode, setMode} = useTheme();

    // 挂载前渲染一个占位，避免服务端渲染 ProLayout（否则会产生响应式 class 不一致）。
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
                    title: '主机列表',
                }}
            >
                <div className="mx-auto max-w-2xl">
                    <HostCard/>
                </div>
            </PageContainer>
        </ProLayout>
    );
}
