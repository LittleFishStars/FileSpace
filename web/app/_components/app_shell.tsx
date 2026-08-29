'use client'

import React, {useSyncExternalStore} from 'react';
import {Breadcrumb, Segmented} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
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

/** 品牌标志：互联网络标识，用 currentColor 随主题（浅/深）自适应 */
function BrandMark({onClick}: {onClick?: () => void}) {
    return (
        <svg
            viewBox="74 85 390 383"
            className="h-7 w-7 cursor-pointer"
            role="img"
            aria-label="文件空间 FileSpace"
            onClick={(e) => {
                // 仅响应 logo 图形本身的点击（切换主题），
                // 不冒泡触发 ProLayout 的返回主页回调（标题文字点击仍返回主页）
                e.stopPropagation();
                onClick?.();
            }}
        >
            <title>点击切换亮暗主题</title>
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

/** 顶部导航菜单：局域网节点（/nodes）与本机节点（/local），所有页面顶栏可见、可随时切换 */
const TOP_MENU = {
    path: '/',
    routes: [
        {path: '/nodes', name: '局域网节点'},
        {path: '/local', name: '本机节点'},
    ],
};

/**
 * 应用外壳：ProLayout 顶部导航模式。
 * 顶栏左侧为 logo + 标题（点击返回主页），右侧为顶部导航菜单
 * （局域网节点 / 本机节点，高亮当前路由）+ 主题切换按钮。
 * 标题可为纯文本（title），也可为面包屑形式（breadcrumb，
 * 形如「局域网节点 > 主机名 > 文件夹」，此时 title 作为加载中的回退标题）。
 */
export default function AppShell({
    title,
    breadcrumb,
    wide = false,
    menuActivePath,
    children,
}: {
    /** 页面标题（未提供 breadcrumb 或 breadcrumb 为空时的回退标题） */
    title?: string;
    /** 面包屑标题项，渲染在标题位置 */
    breadcrumb?: BreadcrumbItem[];
    /** 内容区是否放宽（文件浏览等表格页用），默认窄栏 */
    wide?: boolean;
    /** 覆盖顶部菜单高亮路径（如 /folders 页按文件夹归属标记本机 / 远程）；缺省取当前路由 */
    menuActivePath?: string;
    children: React.ReactNode;
}) {
    const mounted = useSyncExternalStore(subscribeNoop, getClientSnapshot, getServerSnapshot);
    const {mode, isDark, setMode} = useTheme();
    const router = useRouter();
    const pathname = usePathname();

    // 点击 logo：在当前亮/暗主题之间切换（基于实际显示取反，并固化为显式 light/dark 偏好）
    const toggleTheme = () => setMode(isDark ? 'light' : 'dark');

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

    return (
        <ProLayout
            title="文件空间"
            logo={<BrandMark onClick={toggleTheme}/>}
            layout="top"
            route={TOP_MENU}
            location={{pathname: menuActivePath ?? pathname}}
            onMenuHeaderClick={() => router.push('/')}
            menuItemRender={(item, dom) => <Link href={item.path ?? '/'}>{dom}</Link>}
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
