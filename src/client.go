package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func forwardClientArgs(client string, args []string, log *logger) error {
	target := filepath.Join(client, "BaiduNetdisk.exe")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("找不到原版客户端: %w", err)
	}
	cmd := exec.Command(target, args...)
	cmd.Dir = client
	log.Printf("转发客户端参数: %s %v", filepath.Base(target), args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动客户端失败: %w", err)
	}
	return nil
}
