package sync

import (
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
		host    string
		port    int
		folder  string
	}{
		{in: "192.168.1.5:abcd1234", host: "192.168.1.5", port: 0, folder: "abcd1234"},
		{in: "192.168.1.5:9000:abcd5678", host: "192.168.1.5", port: 9000, folder: "abcd5678"},
		{in: "192.168.1.5:123:abcd1234", host: "192.168.1.5", port: 123, folder: "abcd1234"},
		{in: "[::1]:abcd1234", host: "::1", port: 0, folder: "abcd1234"},
		{in: "[::1]:9000:abcd1234", host: "::1", port: 9000, folder: "abcd1234"},
		{in: "myserver.local:abcd1234", host: "myserver.local", port: 0, folder: "abcd1234"},
		// 错误用例
		{in: "", wantErr: true},
		{in: "192.168.1.5", wantErr: true},                // 缺文件夹 id
		{in: "192.168.1.5:zzzzzzzz", wantErr: true},       // 非 hex 文件夹 id
		{in: "192.168.1.5:127.0.0.1:abcd", wantErr: true}, // 端口位置不是数字
		{in: "192.168.1.5:99999:abcd1234", wantErr: true}, // 端口超界
		{in: "a:b:abc", wantErr: true},                    // 文件夹 id 长度不对
	}
	for _, c := range cases {
		s, err := ParseRemote(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRemote(%q) 应报错", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRemote(%q) 意外报错: %v", c.in, err)
			continue
		}
		if s.Host != c.host || s.Port != c.port || s.FolderID != c.folder {
			t.Errorf("ParseRemote(%q) = %+v，期望 host=%q port=%d folder=%q", c.in, s, c.host, c.port, c.folder)
		}
	}
}

// 默认端口与默认地址组合。
func TestSpecAddress(t *testing.T) {
	s := Spec{Host: "10.0.0.2", FolderID: "abcd1234"}
	if got := s.Address(); got != "10.0.0.2:8080" {
		t.Errorf("Address() 默认端口 = %q，期望 10.0.0.2:8080", got)
	}
	s.Port = 9000
	if got := s.Address(); got != "10.0.0.2:9000" {
		t.Errorf("Address() 指定端口 = %q，期望 10.0.0.2:9000", got)
	}
}
