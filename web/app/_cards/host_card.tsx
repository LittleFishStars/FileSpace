'use client'

import Link from 'next/link'
import { ProCard } from '@ant-design/pro-components'
import { Badge, Tooltip } from 'antd'
import {
  ApiOutlined,
  CloudServerOutlined,
  FolderOpenOutlined,
  LaptopOutlined,
  LockOutlined,
} from '@ant-design/icons'
import FolderCard, { type FolderInfo } from './folder_card'
import NodeInfoCard from './node_info_card'

/**
 * 节点卡片。
 * 展示某个节点（主机）的基本信息：主机名、IP、操作系统、软件版本、运行时间，
 * 以及该节点共享的文件夹；共享文件夹以文件夹卡片的形式纵向堆叠展示。
 * 布局：节点信息在上，文件夹卡片在下。
 */
export interface HostInfo {
  id: string
  hostname: string
  ip: string
  os: string
  status: 'online' | 'offline'
  uptime: string
  /** 节点上运行的程序版本 */
  softwareVersion: string
  /** 该节点是否设置了共享访问密码（访问其文件需输入密码） */
  auth?: boolean
  folders: FolderInfo[]
}

export default function HostCard({ host }: { host: HostInfo }) {
  const online = host.status === 'online'

  return (
    <ProCard
      bordered
      className="w-full"
      headerBordered
      title={
        <span className="flex items-center gap-2 text-base font-semibold">
          <CloudServerOutlined className="text-neutral-500 dark:text-neutral-400" />
          {host.hostname}
          {host.auth && (
            <Tooltip title="该节点有需要密码访问的共享文件夹">
              <LockOutlined className="text-amber-500 dark:text-amber-400" />
            </Tooltip>
          )}
        </span>
      }
      extra={
        <Badge
          status={online ? 'success' : 'error'}
          text={online ? '在线' : '离线'}
        />
      }
      bodyStyle={{ padding: 16 }}
    >
      {/* 节点标识 + 共享文件夹数 */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-100 text-blue-600 dark:bg-blue-400/20 dark:text-blue-400">
            <LaptopOutlined className="text-2xl" />
          </div>
          <div>
            <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
              {host.hostname}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">
              <ApiOutlined className="mr-1" />
              {host.ip}
            </div>
          </div>
        </div>
        <div className="text-right">
          <div className="text-xl font-semibold text-neutral-900 dark:text-neutral-100">
            {host.folders.length}
          </div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">共享文件夹</div>
        </div>
      </div>

      {/* 节点信息卡片（默认折叠） */}
      <NodeInfoCard
        info={{
          ip: host.ip,
          os: host.os,
          softwareVersion: host.softwareVersion,
          uptime: host.uptime,
        }}
      />

      {/* 共享文件夹区 */}
      <div className="flex items-center gap-2 border-t border-neutral-200 pt-4 dark:border-neutral-700/60">
        <FolderOpenOutlined className="text-amber-500 dark:text-amber-400" />
        <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
          共享文件夹
        </span>
        <span className="text-xs text-neutral-500 dark:text-neutral-400">
          共 {host.folders.length} 个
        </span>
      </div>
      <div className="mt-3 flex flex-col gap-3">
        {host.folders.map((folder) => (
          <Link key={folder.id} href={`/folders?folderId=${folder.id}`} className="block">
            <FolderCard folder={folder}/>
          </Link>
        ))}
      </div>
    </ProCard>
  )
}
