'use client'

import React, {useSyncExternalStore} from 'react';
import {Breadcrumb, Segmented} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import Link from 'next/link';
import {useRouter} from 'next/navigation';
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

/** 面包屑项：title 为显示内容，href 存在时渲染为可点击链接 */
export interface BreadcrumbItem {
    title: React.ReactNode;
    href?: string;
}

/** 顶栏选项卡：渲染在标题与主题切换按钮所在的那一行 */
export interface TopTabItem {
    key: string;
    label: React.ReactNode;
}

/** 品牌标志：互联网络标识，用 currentColor 随主题（浅/深）自适应 */
function BrandMark() {
    return (
        <svg viewBox="74 85 390 383" className="h-7 w-7" aria-hidden>
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

/** 主题切换选项（模块级常量，避免每次渲染重建数组） */
const THEME_OPTIONS = [
    {label: '系统', value: 'system' as ThemeMode, icon: <DesktopOutlined/>},
    {label: '浅色', value: 'light' as ThemeMode, icon: <SunOutlined/>},
    {label: '深色', value: 'dark' as ThemeMode, icon: <MoonOutlined/>},
];

/**
 * 应用外壳：ProLayout 顶部导航 + 右上角主题切换 + 页面容器。
 * 各页面共用；标题可为纯文本（title），也可为面包屑形式（breadcrumb，
 * 形如「主机列表 > 主机名 > 文件夹」，此时 title 作为加载中的回退标题）。
 */
export default function AppShell({
    title,
    breadcrumb,
    wide = false,
    topTabs,
    children,
}: {
    /** 页面标题（未提供 breadcrumb 或 breadcrumb 为空时的回退标题） */
    title?: string;
    /** 面包屑标题项，渲染在标题位置 */
    breadcrumb?: BreadcrumbItem[];
    /** 内容区是否放宽（文件浏览等表格页用），默认窄栏 */
    wide?: boolean;
    /** 顶栏选项卡（可选）：渲染在标题 / 主题切换按钮那一行，切换由页面自身控制 */
    topTabs?: {
        items: TopTabItem[];
        activeKey: string;
        onChange: (key: string) => void;
    };
    children: React.ReactNode;
}) {
    const mounted = useSyncExternalStore(subscribeNoop, getClientSnapshot, getServerSnapshot);
    const {mode, setMode} = useTheme();
    const router = useRouter();

    // 挂载前渲染占位，避免服务端渲染 ProLayout。
    if (!mounted) {
        return <div className="min-h-screen"/>;
    }

    // 有面包屑时标题位置渲染为面包屑，否则回退为纯文本标题。
    const header =
        breadcrumb && breadcrumb.length > 0
            ? {
                  title: (
                      <Breadcrumb
                          separator=">"
                          items={breadcrumb.map((item) => ({
                              title: item.href ? <Link href={item.href}>{item.title}</Link> : item.title,
                          }))}
                      />
                  ),
              }
            : {title};

    // 顶栏左侧：logo + 标题文字（+ 可选选项卡）组合为一个节点，
    // 点击返回主页（ProLayout 的 title 仅接受字符串，故用 logo 承载组合内容并关闭默认标题）。
    // 选项卡点击需阻止冒泡，避免触发返回主页的跳转。
    const titleNode = false;
    const logoNode = (
        <span className="flex cursor-pointer items-center gap-3" onClick={() => router.push('/')}>
            <BrandMark/>
            <span className="text-base font-semibold text-neutral-900 dark:text-neutral-100">文件空间</span>
            {topTabs && (
                <span onClick={(e) => e.stopPropagation()}>
                    <Segmented
                        size="middle"
                        value={topTabs.activeKey}
                        onChange={(value) => topTabs.onChange(value as string)}
                        options={topTabs.items.map((item) => ({label: item.label, value: item.key}))}
                    />
                </span>
            )}
        </span>
    );

    return (
        <ProLayout
            title={titleNode}
            logo={logoNode}
            layout={'top'}
            actionsRender={() => [
                <Segmented
                    key="theme-toggle"
                    size="middle"
                    style={{marginRight: 16}}
                    value={mode}
                    onChange={(value) => setMode(value as ThemeMode)}
                    options={THEME_OPTIONS}
                />,
            ]}
        >
            <PageContainer header={header}>
                <div className={`mx-auto w-full ${wide ? 'max-w-5xl' : 'max-w-2xl'}`}>
                    {children}
                </div>
            </PageContainer>
        </ProLayout>
    );
}
