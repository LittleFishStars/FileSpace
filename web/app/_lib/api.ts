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
    /** 该节点是否设置了共享访问密码（远程访问需先认证） */
    auth: boolean
    /** 当前请求是否来自本机（回环访问）：远程访问时本机节点按局域网节点展示 */
    local: boolean
}

export interface ApiFolderInfo {
    id: string
    name: string
    path: string
    fileCount: number
    totalSize: number
    updatedAt: string
    /** 该文件夹是否设置了访问密码（远程访问需先认证） */
    auth: boolean
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

/** API 请求错误：附带 HTTP 状态码（如 401，供调用方触发重新认证） */
export class ApiError extends Error {
    status: number

    constructor(url: string, status: number, detail = '') {
        super(detail ? `${url} 返回 ${status}: ${detail}` : `${url} 返回 ${status}`)
        this.status = status
    }
}

/** 带访问令牌的请求头（无令牌时返回 undefined，走普通请求） */
function authInit(token?: string): RequestInit | undefined {
    return token ? {headers: {Authorization: `Bearer ${token}`}} : undefined
}

/** 发起 GET 请求并解析 JSON，非 2xx 抛 ApiError */
async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
    const res = await fetch(url, init)
    if (!res.ok) {
        throw new ApiError(url, res.status)
    }
    return res.json() as Promise<T>
}

/** 发起 JSON POST 请求并解析 JSON，非 2xx 抛 ApiError（错误响应体带 error 字段时一并展示） */
async function postJSON<T>(url: string, body?: unknown): Promise<T> {
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
        throw new ApiError(url, res.status, detail)
    }
    return res.json() as Promise<T>
}

export const fetchNode = () => fetchJSON<ApiNodeInfo>('/api/node')

export const fetchFolders = () => fetchJSON<ApiFolderInfo[]>('/api/folders')

export const fetchPeers = () => fetchJSON<ApiPeerInfo[]>('/api/peers')

/** 追加共享目录（仅本机调用生效），password 为该文件夹的可选访问密码（空表示开放） */
export const addFolders = (paths: string[], password = '') =>
    postJSON<{added: ApiFolderInfo[]}>('/api/folders/add', {paths, password}).then((r) => r.added)

/** 移除共享目录（仅本机调用生效） */
export const removeFolder = (id: string) =>
    postJSON<{removed: string}>('/api/folders/remove', {id}).then((r) => r.removed)

/** 修改/移除本机共享文件夹的访问密码（password 为空表示移除密码、恢复开放；仅本机调用生效） */
export const setFolderPassword = (path: string, password: string) =>
    postJSON<{updated: string; name: string; auth: boolean}>('/api/folders/password', {path, password})

/**
 * 调用后端在本机弹出系统原生目录选择对话框（仅本机回环调用生效），
 * 返回所选目录的绝对路径；cancelled 表示用户取消了选择。
 * 浏览器出于安全限制不向网页暴露所选文件夹的绝对路径，故由后端弹系统对话框并直接取得路径。
 */
export const pickDirectory = () =>
    postJSON<{path?: string; cancelled?: boolean}>('/api/local/pick-directory')

/** 用系统默认应用打开本机共享目录内的文件（仅本机调用生效） */
export const openFile = (folderId: string, path: string) =>
    postJSON<{opened: string}>(`/api/folders/${folderId}/open?path=${encodeURIComponent(path)}`)

/**
 * 懒加载共享目录内的文件列表。
 * base 为文件夹所属节点的 API 基地址：本机为空字符串（同源/反代），
 * 远端节点为 http://<ip>:<port>（该文件夹由远端后端管理，需直连其 API）。
 * token 为远端节点设置的访问密码换取的访问令牌（该节点未设密码时省略）。
 */
export const fetchTree = (folderId: string, path: string, base = '', token?: string) =>
    fetchJSON<ApiFileInfo[]>(
        `${base}/api/folders/${folderId}/tree?path=${encodeURIComponent(path)}`,
        authInit(token),
    )

/** 生成文件下载/查看 URL（base 语义同 fetchTree；token 通过 query 传递，供 <a href> 下载使用） */
export const downloadUrl = (folderId: string, path: string, base = '', token?: string) =>
    `${base}/api/folders/${folderId}/download?path=${encodeURIComponent(path)}${token ? `&token=${encodeURIComponent(token)}` : ''}`

/**
 * 向远端节点提交共享访问密码（POST /api/auth），成功返回访问令牌。
 * 该节点未设置密码时返回 404 抛错。
 */
export const authLogin = (base: string, password: string) =>
    postJSON<{token: string}>(`${base}/api/auth`, {password}).then((r) => r.token)
