'use client'

import React from 'react'
import { Collapse } from 'antd'
import {
  ClockCircleOutlined,
  ClusterOutlined,
  LaptopOutlined,
  TagsOutlined,
  WifiOutlined,
} from '@ant-design/icons'

/**
 * 节点信息卡片。
 * 抽象出节点的运行信息：IP、操作系统、软件版本、运行时间。
 * 默认以折叠卡片的形式呈现（collapsible=true，局域网节点卡片用）；
 * 本机节点等场景可关闭折叠（collapsible=false），直接平铺展示。
 */
export interface NodeInfo {
  ip: string
  os: string
  /** 节点上运行的程序版本 */
  softwareVersion: string
  uptime: string
}

/** 单条信息项（图标 + 标签 + 数值） */
function InfoItem({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode
  label: string
  value: React.ReactNode
}) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-neutral-200 bg-neutral-50 px-4 py-3 dark:border-neutral-700/60 dark:bg-neutral-800/40">
      <span className="text-lg text-neutral-400 dark:text-neutral-500">{icon}</span>
      <div className="min-w-0">
        <div className="text-xs text-neutral-500 dark:text-neutral-400">{label}</div>
        <div className="truncate text-sm font-medium text-neutral-900 dark:text-neutral-100">
          {value}
        </div>
      </div>
    </div>
  )
}

export default function NodeInfoCard({
  info,
  collapsible = true,
}: {
  info: NodeInfo
  /** 是否以折叠卡片呈现（默认折叠）；false 时直接平铺 */
  collapsible?: boolean
}) {
  const infoItems = [
    { icon: <WifiOutlined />, label: 'IP 地址', value: info.ip },
    { icon: <LaptopOutlined />, label: '操作系统', value: info.os },
    { icon: <TagsOutlined />, label: '软件版本', value: info.softwareVersion },
    { icon: <ClockCircleOutlined />, label: '运行时间', value: info.uptime },
  ]

  // 不折叠：直接平铺，大屏一行四个、中屏两行两个、小屏四行一个
  if (!collapsible) {
    return (
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-4">
        {infoItems.map((item) => (
          <InfoItem key={item.label} icon={item.icon} label={item.label} value={item.value} />
        ))}
      </div>
    )
  }

  return (
    <Collapse
      className="mb-4"
      ghost
      defaultActiveKey={[]}
      items={[
        {
          key: 'node-info',
          label: (
            <span className="flex items-center gap-2 text-sm font-medium text-neutral-900 dark:text-neutral-100">
              <ClusterOutlined className="text-neutral-500 dark:text-neutral-400" />
              节点信息
              <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">
                {infoItems.length} 项
              </span>
            </span>
          ),
          children: (
            <div className="grid grid-cols-1 gap-2 pt-1 sm:grid-cols-2 xl:grid-cols-4">
              {infoItems.map((item) => (
                <InfoItem key={item.label} icon={item.icon} label={item.label} value={item.value} />
              ))}
            </div>
          ),
        },
      ]}
    />
  )
}
