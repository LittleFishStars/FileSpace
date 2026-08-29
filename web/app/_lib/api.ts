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

/** 发起 JSON POST 请求并解析 JSON，非 2xx 抛错（错误响应体带 error 字段时一并展示） */
export async function postJSON<T>(url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: body !== undefined ? {'Content-Type': 'application/json'} : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    let detail = ''
    try {
      const data = (await res.json()) as {error?: string}
      detail = data.error ?? ''
    } catch {
      // 非 JSON 错误体，忽略
    }
    throw new Error(detail ? `${url} 返回 ${res.status}: ${detail}` : `${url} 返回 ${res.status}`)
  }
  return res.json() as Promise<T>
}

export const fetchNode = () => fetchJSON<ApiNodeInfo>('/api/node')

export const fetchFolders = () => fetchJSON<ApiFolderInfo[]>('/api/folders')

export const fetchPeers = () => fetchJSON<ApiPeerInfo[]>('/api/peers')

/** 追加共享目录（仅本机调用生效），返回新增的文件夹列表 */
export const addFolders = (paths: string[]) =>
  postJSON<{added: ApiFolderInfo[]}>('/api/folders/add', {paths}).then((r) => r.added)

/** 移除共享目录（仅本机调用生效） */
export const removeFolder = (id: string) =>
  postJSON<{removed: string}>('/api/folders/remove', {id}).then((r) => r.removed)

/** 用系统默认应用打开本机共享目录内的文件（仅本机调用生效） */
export const openFile = (folderId: string, path: string) =>
  postJSON<{opened: string}>(`/api/folders/${folderId}/open?path=${encodeURIComponent(path)}`)

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
