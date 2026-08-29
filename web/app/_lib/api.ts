// 后端 API 数据模型与请求封装（与 backend/internal/model 对齐）

export interface ApiNodeInfo {
  id: string
  hostname: string
  ip: string
  os: string
  softwareVersion: string
  status: string
  uptime: string
  listenAddr: string
}

export interface ApiFolderInfo {
  id: string
  name: string
  path: string
  fileCount: number
  totalSize: number
  updatedAt: string
}

export interface ApiFileInfo {
  name: string
  path: string
  size: number
  modTime: string
  isDir: boolean
  /** 是否可在线预览（后端检测：文本类或 FileViewer 可渲染） */
  previewable: boolean
}

export interface ApiPeerInfo {
  node: ApiNodeInfo
  folders: ApiFolderInfo[]
  online: boolean
  lastSeen: string
}

/** 发起 GET 请求并解析 JSON，非 2xx 抛错 */
export async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`${url} 返回 ${res.status}`)
  }
  return res.json() as Promise<T>
}

export const fetchNode = () => fetchJSON<ApiNodeInfo>('/api/node')

export const fetchFolders = () => fetchJSON<ApiFolderInfo[]>('/api/folders')

export const fetchPeers = () => fetchJSON<ApiPeerInfo[]>('/api/peers')

/**
 * 懒加载共享目录内的文件列表。
 * base 为文件夹所属节点的 API 基地址：本机为空字符串（同源/反代），
 * 远端节点为 http://<ip>:<port>（该文件夹由远端后端管理，需直连其 API）。
 */
export const fetchTree = (folderId: string, path: string, base = '') =>
  fetchJSON<ApiFileInfo[]>(`${base}/api/folders/${folderId}/tree?path=${encodeURIComponent(path)}`)

/** 生成文件下载/查看 URL（base 语义同 fetchTree） */
export const downloadUrl = (folderId: string, path: string, base = '') =>
  `${base}/api/folders/${folderId}/download?path=${encodeURIComponent(path)}`
