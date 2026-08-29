// Package model 定义与前端对齐的数据模型。
package model

// NodeInfo 节点信息。
type NodeInfo struct {
	ID              string `json:"id"`
	Hostname        string `json:"hostname"`
	IP              string `json:"ip"`
	OS              string `json:"os"`
	SoftwareVersion string `json:"softwareVersion"`
	Status          string `json:"status"` // online / offline
	Uptime          string `json:"uptime"`
	ListenAddr      string `json:"listenAddr"`
	// Auth 该节点是否设置了共享访问密码：远程节点需先通过 /api/auth 认证
	// 才能查看/下载本节点共享的文件内容（本机回环访问不受影响）。
	Auth bool `json:"auth"`
}

// FolderInfo 共享文件夹信息。
type FolderInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	FileCount int    `json:"fileCount"`
	TotalSize int64  `json:"totalSize"` // 字节
	UpdatedAt string `json:"updatedAt"`
	// Auth 该文件夹是否设置了访问密码：远程访问需先认证才能查看/下载内容（本机回环豁免）。
	Auth bool `json:"auth"`
}

// FileInfo 文件（或目录）信息。
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
	// Previewable 是否可在线预览：文本类（泛指所有能以文本读取的文件，
	// 含代码/CSV/JSON/Markdown/HTML/XML 等）或非文本但 FileViewer 可渲染的（图片/视频/PDF/Office）。
	Previewable bool `json:"previewable"`
}

// PeerInfo 发现的其他节点（含其共享文件夹）。
type PeerInfo struct {
	Node     NodeInfo     `json:"node"`
	Folders  []FolderInfo `json:"folders"`
	Online   bool         `json:"online"`
	LastSeen string       `json:"lastSeen"`
}
