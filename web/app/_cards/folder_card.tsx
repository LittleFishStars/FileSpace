'use client'

import { FolderOutlined, LockOutlined, SyncOutlined } from '@ant-design/icons'
import { ProCard } from '@ant-design/pro-components'
import { App as AntdApp, Button, Tooltip } from 'antd'
import { formatSize, formatTime } from '../_lib/format'
import { copyText } from '../_lib/clipboard'
import type { ApiFolderInfo } from '../_lib/api'

/**
 * 文件夹卡片。
 * 展示共享文件夹的核心信息：文件夹名、文件数与总大小。
 * 直接复用后端的 ApiFolderInfo 数据模型（不另造精简接口，避免重复映射）。
 * onSync 存在时在卡片右侧渲染「同步」按钮（把该文件夹同步到本机）。
 */
export default function FolderCard({
  folder,
  className,
  onSync,
}: {
  folder: ApiFolderInfo
  className?: string
  /** 提供同步入口时渲染「同步」按钮（点击把远程共享文件夹同步到本机） */
  onSync?: () => void
}) {
  // 全量统计：文件数与总大小由后端后台扫描缓存（目录较大时首次显示可能有短暂延迟）
  const stats = [
    { label: '文件', value: String(folder.fileCount) },
    { label: '总大小', value: formatSize(folder.totalSize) },
  ]
  const { message } = AntdApp.useApp()

  /** 点击复制文件夹 ID（用于 -s/--sync 等需要手动指定文件夹 id 的场景）。
   *  卡片整体被 Link 包裹（host_card 点击进入浏览页），故阻止冒泡与默认跳转 */
  const handleCopyId = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const ok = await copyText(folder.id)
    if (ok) {
      message.success(`已复制文件夹 ID：${folder.id}`)
    } else {
      message.error('复制失败，请手动选择复制')
    }
  }

  return (
    <ProCard
      bordered
      hoverable
      className={className}
      bodyStyle={{ padding: 16 }}
    >
      {/* 头部：图标 + 名称 + 更新时间；右侧为同步入口（可选） */}
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-amber-100 text-amber-600 dark:bg-amber-400/20 dark:text-amber-400">
          <FolderOutlined className="text-2xl" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate text-base font-semibold text-neutral-900 dark:text-neutral-100">
              {folder.name}
            </span>
            <Tooltip title="点击复制文件夹 ID">
              <span
                className="shrink-0 cursor-pointer rounded px-1 text-xs font-normal text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-600 dark:text-neutral-500 dark:hover:bg-neutral-800 dark:hover:text-neutral-300"
                onClick={handleCopyId}
              >
                {folder.id}
              </span>
            </Tooltip>
            {folder.auth && (
              <Tooltip title="该文件夹设置了访问密码，需输入密码才能访问">
                <LockOutlined className="shrink-0 text-sm text-amber-500 dark:text-amber-400" />
              </Tooltip>
            )}
          </div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">
            {formatTime(folder.updatedAt)} 更新
          </div>
        </div>
        {/* 同步入口：卡片整体被 Link 包裹，阻止冒泡避免触发进入浏览页的跳转 */}
        {onSync && (
          <Button
            type="primary"
            size="small"
            icon={<SyncOutlined />}
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              onSync()
            }}
          >
            同步
          </Button>
        )}
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
