package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsRunningAsAdmin checks if the process is running with root privileges on macOS.
func IsRunningAsAdmin() bool {
	return os.Geteuid() == 0
}

// AppDataDir returns the DirectLink config directory on macOS.
func AppDataDir() string {
	dir := filepath.Join(os.Getenv("HOME"), ".directlink")
	os.MkdirAll(dir, 0755)
	return dir
}

// SetAutoStart creates a LaunchAgent plist for auto-start on login.
func SetAutoStart(exePath string) error {
	plistDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	os.MkdirAll(plistDir, 0755)

	plistPath := filepath.Join(plistDir, "com.directlink.accelerator.plist")

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.directlink.accelerator</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>`, exePath)

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("写入 LaunchAgent 失败: %w", err)
	}
	return nil
}

// RemoveAutoStart removes the LaunchAgent plist.
func RemoveAutoStart() error {
	plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.directlink.accelerator.plist")
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 LaunchAgent 失败: %w", err)
	}
	return nil
}

// IsAutoStartEnabled checks if the LaunchAgent exists.
func IsAutoStartEnabled() bool {
	plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.directlink.accelerator.plist")
	_, err := os.Stat(plistPath)
	return err == nil
}

// ReadAutoStartValue reads the executable path from the LaunchAgent plist.
func ReadAutoStartValue() (string, error) {
	// Not needed on macOS, return empty
	return "", nil
}

// GetExePath returns the path to the current executable.
func GetExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return exe, nil
}

// init: ensure config dir exists
func init() {
	_ = strings.TrimSpace // keep import
}
