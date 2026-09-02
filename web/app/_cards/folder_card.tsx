'use client'

import React from 'react'
import { FolderOutlined, LockOutlined } from '@ant-design/icons'
import { ProCard } from '@ant-design/pro-components'
import { Tooltip } from 'antd'

/**
 * 文件夹卡片。
 * 展示共享文件夹的核心信息：文件夹名、文件数与总大小。
 * 布局参考手绘稿：名称 + 顶部图标，下方为统计信息行。
 */
export interface FolderInfo {
  id: string
  name: string
  /** 总大小（字节） */
  totalSize: number
  /** 文件数量（可选） */
  fileCount?: number
  /** 最近更新时间（可选） */
  updatedAt?: string
  /** 该文件夹是否设置了访问密码（远程访问需先认证） */
  auth?: boolean
}

/** 将字节数格式化为可读大小 */
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export default function FolderCard({
  folder,
  className,
}: {
  folder: FolderInfo
  className?: string
}) {
  // 全量统计：文件数与总大小由后端后台扫描缓存（目录较大时首次显示可能有短暂延迟）
  const stats = [
    { label: '文件', value: String(folder.fileCount ?? 0) },
    { label: '总大小', value: formatSize(folder.totalSize) },
  ]

  return (
    <ProCard
      bordered
      hoverable
      className={className}
      bodyStyle={{ padding: 16 }}
    >
      {/* 头部：图标 + 名称 + 更新时间 */}
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-amber-100 text-amber-600 dark:bg-amber-400/20 dark:text-amber-400">
          <FolderOutlined className="text-2xl" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate text-base font-semibold text-neutral-900 dark:text-neutral-100">
              {folder.name}
            </span>
            {folder.auth && (
              <Tooltip title="该文件夹设置了访问密码，需输入密码才能访问">
                <LockOutlined className="shrink-0 text-sm text-amber-500 dark:text-amber-400" />
              </Tooltip>
            )}
          </div>
          {folder.updatedAt && (
            <div className="text-xs text-neutral-500 dark:text-neutral-400">
              {folder.updatedAt} 更新
            </div>
          )}
        </div>
      </div>

      {/* 统计信息行 */}
      <div className="mt-4 flex items-center justify-between border-t border-neutral-200 pt-3 dark:border-neutral-700/60">
        {stats.map((stat) => (
          <div key={stat.label}>
            <div className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
              {stat.value}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">
              {stat.label}
            </div>
          </div>
        ))}
      </div>
    </ProCard>
  )
}
