//go:build windows

package api

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// pickDirectoryDialog 在 Windows 上通过 PowerShell 的 FolderBrowserDialog
// 弹出原生目录选择对话框，返回用户选择的绝对路径；用户取消时返回空字符串与 nil 错误。
// 路径以 Base64(UTF-8) 形式输出，规避 PowerShell 输出编码（如 GBK）导致的中文路径乱码。
func pickDirectoryDialog() (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '选择要共享的文件夹'
$d.ShowNewFolderButton = $false
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($d.SelectedPath)
    Write-Output ([System.Convert]::ToBase64String($bytes))
}
`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script).Output()
	if err != nil {
		if strings.TrimSpace(string(out)) == "" {
			return "", nil // 用户取消
		}
		return "", err
	}
	b64 := strings.TrimSpace(string(out))
	if b64 == "" {
		return "", nil // 用户取消
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("解析所选路径失败: %w", err)
	}
	return string(data), nil
}
