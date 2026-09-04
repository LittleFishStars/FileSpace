package share

import (
	"filespace/internal/auth"
	"filespace/internal/config"
)

// passwdHashOf 取共享配置项的密码哈希：优先用持久化的 PasswdHash（十六进制字符串），
// 缺失则对明文 Passwd 现场哈希。两者皆空得到零值 Hash（表示该文件夹开放）。
func passwdHashOf(sf config.SharedFolder) auth.Hash {
	if h, ok := auth.ParseHex(sf.PasswdHash); ok {
		return h
	}
	return auth.OfPassword(sf.Passwd)
}

// ApplyDefaultPasswdHash 用默认密码填充未显式设置密码的共享目录配置项：
// 写入其 PasswdHash（十六进制，与持久化表示一致），
// 不覆盖配置文件中为单个文件夹指定的独立密码（含其哈希表示）。
// 供启动阶段（cmd）在解析共享列表后调用。
func ApplyDefaultPasswdHash(shared []config.SharedFolder, passwd string) {
	hash := auth.OfPassword(passwd).Hex()
	if hash == "" {
		return // 无默认密码：保持各文件夹原有（可能为空的）密码
	}
	for i := range shared {
		if shared[i].Passwd == "" && shared[i].PasswdHash == "" {
			shared[i].PasswdHash = hash
		}
	}
}
