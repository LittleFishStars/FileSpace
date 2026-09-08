// Package sync 实现「同步文件夹」：把远程 filespace 节点的某个共享文件夹内容
// 单向镜像到本地目录（后台常驻、周期对账）。
//
// 由命令行 -s/--sync <远程ip:文件夹id> <本地路径> 创建：远程文件夹 id 是该文件夹
// 路径在共享时生成的稳定哈希（见 internal/share 的 folderID），与远程 /api/folders
// 返回的 id 一致。同步基于远程公开 API（/api/folders 定位 + /api/auth 认证 +
// /api/folders/{id}/tree 懒加载列举 + /api/folders/{id}/download 下载）实现，
// 因此本包只依赖 internal/model，不依赖同进程内的共享管理。
//
// 语义：远程为事实源，本地为镜像——
//   - 远端新增/修改 → 增量下载（大小+修改时间相同则跳过，避免重复拉取）；
//   - 远端删除 → 同步删除本地对应条目；
//   - 本地多余条目（远端不存在）→ 删除。
//
// 因此本地同步目录里的改动会被覆盖/清理，与「把远端文件夹镜像到本地」一致。
package sync

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// DefaultRemotePort 远程节点未显式给出端口时的默认值（与后端默认监听端口一致）。
const DefaultRemotePort = 8080

// Spec 一个后台同步任务的配置：远程定位符 + 本地同步目录。
type Spec struct {
	// Host 远程节点地址（不带端口）：IP 或主机名；IPv6 用方括号包裹传入后剥离。
	Host string `json:"host"`
	// Port 远程节点监听端口；0 表示使用默认端口 8080。
	Port int `json:"port"`
	// FolderID 远程节点上要同步的共享文件夹 id（由该文件夹路径生成，8 位十六进制）。
	FolderID string `json:"folder_id"`
	// Local 本地同步文件夹路径（不存在则自动创建）。
	Local string `json:"local"`
	// Passwd 访问远端该文件夹所用的密码（该文件夹设置了访问密码时使用；空表示开放）。
	Passwd string `json:"passwd,omitempty"`
}

// Address 返回远程节点地址 host:port（未指定端口时补默认端口）。
func (s Spec) Address() string {
	port := DefaultRemotePort
	if s.Port != 0 {
		port = s.Port
	}
	return net.JoinHostPort(s.Host, strconv.Itoa(port))
}

// isFolderID 校验文件夹 id：share.folderID 用 fnv32a 对路径哈希后 hex 编码为 8 位十六进制。
func isFolderID(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// ParseRemote 解析 -s/--sync 的参数值（"远程ip:文件夹id" 或 "远程ip:端口:文件夹id"；
// IPv6 用 [addr] 包裹，如 "[::1]:文件夹id"、"192.168.1.5:9000:abcd1234"）。
// 返回 Spec 基础信息（Host/Port/FolderID），Local 与 Passwd 由调用方补充。
func ParseRemote(arg string) (Spec, error) {
	spec := Spec{}
	rest := strings.TrimSpace(arg)
	if rest == "" {
		return spec, fmt.Errorf("同步定位符为空")
	}

	if strings.HasPrefix(rest, "[") {
		// 方括号包裹的 IPv6 地址：先剥离 [addr]
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return spec, fmt.Errorf("IPv6 地址缺少闭合方括号: %s", arg)
		}
		spec.Host = rest[1:end]
		rest = strings.TrimPrefix(rest[end+1:], ":")
	} else {
		// 其余形式：host 取第一个 ':' 之前的部分
		idx := strings.IndexByte(rest, ':')
		if idx < 0 {
			return spec, fmt.Errorf("同步定位符缺少文件夹 id，应为 <ip>:<文件夹id>: %s", arg)
		}
		spec.Host = rest[:idx]
		rest = rest[idx+1:]
	}
	if spec.Host == "" {
		return spec, fmt.Errorf("同步定位符缺少远程节点地址: %s", arg)
	}

	// 剩余部分：可能是 "<folderid>"，或 "<端口>:<folderid>"
	parts := strings.Split(rest, ":")
	switch len(parts) {
	case 1:
		spec.FolderID = parts[0]
	case 2:
		port, err := strconv.Atoi(parts[0])
		if err != nil || port <= 0 || port > 65535 {
			return spec, fmt.Errorf("同步定位符的端口无效: %q", parts[0])
		}
		spec.Port = port
		spec.FolderID = parts[1]
	default:
		return spec, fmt.Errorf("同步定位符格式错误（仅支持 <ip>:<id> 或 <ip>:<端口>:<id>）: %s", arg)
	}

	if !isFolderID(spec.FolderID) {
		return spec, fmt.Errorf("文件夹 id 无效（应为该文件夹路径生成的 8 位十六进制）: %q", spec.FolderID)
	}
	return spec, nil
}
