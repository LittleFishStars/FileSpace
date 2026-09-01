'use client'

import React, {useEffect, useState, useSyncExternalStore} from 'react';
import {Breadcrumb, Segmented} from 'antd';
import {DesktopOutlined, MoonOutlined, SunOutlined} from '@ant-design/icons';
import Link from 'next/link';
import {usePathname, useRouter} from 'next/navigation';
import {PageContainer, ProLayout} from '@ant-design/pro-components';
import BrandMark from './brand_mark';
import {useTheme, type ThemeMode} from './app_theme';
import {fetchNode} from '../_lib/api';

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

/** 主题切换选项（模块级常量，避免每次渲染重建数组） */
const THEME_OPTIONS = [
    {label: '系统', value: 'system' as ThemeMode, icon: <DesktopOutlined/>},
    {label: '浅色', value: 'light' as ThemeMode, icon: <SunOutlined/>},
    {label: '深色', value: 'dark' as ThemeMode, icon: <MoonOutlined/>},
];

/** 顶部导航菜单路由模板：远程访问时隐藏「本机节点」（本机节点按局域网节点展示） */
function buildMenu(isLocalAccess: boolean) {
    return {
        path: '/',
        routes: [
            {path: '/nodes', name: '局域网节点'},
            ...(isLocalAccess ? [{path: '/local', name: '本机节点'}] : []),
        ],
    };
}

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
    const {mode, setMode} = useTheme();
    const router = useRouter();
    const pathname = usePathname();
    // 是否本机（回环）访问：远程访问时隐藏「本机节点」菜单，默认按本机访问渲染，
    // 加载 /api/node 后再按实际访问来源修正。
    const [isLocalAccess, setIsLocalAccess] = useState(true);

    useEffect(() => {
        let cancelled = false;
        fetchNode()
            .then((node) => {
                if (!cancelled) setIsLocalAccess(node.local !== false);
            })
            .catch(() => {
                // 拉取失败保持默认（本机访问视图），不影响页面主体功能
            });
        return () => {
            cancelled = true;
        };
    }, []);

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
            logo={<BrandMark className="h-7 w-7"/>}
            layout="top"
            route={buildMenu(isLocalAccess)}
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
