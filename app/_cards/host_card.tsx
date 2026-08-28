'use client'

import { ProCard } from '@ant-design/pro-components'
import { Badge } from 'antd'
import {
  ApiOutlined,
  CloudServerOutlined,
  FolderOpenOutlined,
  LaptopOutlined,
} from '@ant-design/icons'
import FolderCard, { type FolderInfo } from './folder_card'
import NodeInfoCard from './node_info_card'

/**
 * 节点卡片。
 * 展示某个节点（主机）的基本信息：主机名、IP、操作系统、CPU/内存占用、运行时间，
 * 以及该节点共享的文件夹；共享文件夹以文件夹卡片的形式纵向堆叠展示。
 * 布局参考手绘稿：节点信息在上，文件夹卡片在下。
 */
export interface HostInfo {
  id: string
  hostname: string
  ip: string
  os: string
  status: 'online' | 'offline'
  cpuUsage: number
  memUsage: number
  uptime: string
  /** 节点上运行的程序版本 */
  softwareVersion: string
  folders: FolderInfo[]
}

const defaultHost: HostInfo = {
  id: 'node-01',
  hostname: 'file-server',
  ip: '192.168.1.100',
  os: 'Arch Linux x86_64',
  status: 'online',
  cpuUsage: 23,
  memUsage: 8.5,
  uptime: '12 天 4 小时',
  softwareVersion: 'v2.3.1',
  folders: [
    {
      id: 'folder-share',
      name: '共享资料',
      projectCount: 6,
      fileCount: 1284,
      totalSize: 42.5 * 1024 * 1024 * 1024,
      updatedAt: '刚刚',
    },
    {
      id: 'folder-projects',
      name: '项目归档',
      projectCount: 18,
      fileCount: 342,
      totalSize: 12.8 * 1024 * 1024 * 1024,
      updatedAt: '昨天',
    },
    {
      id: 'folder-backup',
      name: '备份空间',
      projectCount: 3,
      fileCount: 96,
      totalSize: 208 * 1024 * 1024 * 1024,
      updatedAt: '3 天前',
    },
  ],
}

export default function HostCard({ host = defaultHost }: { host?: HostInfo }) {
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
          cpuUsage: host.cpuUsage,
          memUsage: host.memUsage,
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
          <FolderCard key={folder.id} folder={folder} />
        ))}
      </div>
    </ProCard>
  )
}
