package system

import (
	"fmt"
	"os/exec"
	"strings"

	"directlink/internal/logger"
)

// getActiveNetworkService returns the active network service name (e.g. "Wi-Fi", "Ethernet").
func getActiveNetworkService() string {
	// Try to get the active network service
	out, err := exec.Command("networksetup", "-listnetworkservices").Output()
	if err != nil {
		return "Wi-Fi" // fallback
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip header and disabled services
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		// Return the first active service
		return strings.TrimPrefix(line, "* ")
	}
	return "Wi-Fi"
}

// SetSystemProxy enables the macOS system HTTP/HTTPS proxy.
func SetSystemProxy(addr string) error {
	service := getActiveNetworkService()
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid proxy address: %s", addr)
	}
	host, port := parts[0], parts[1]

	// Set HTTP proxy
	if err := exec.Command("networksetup", "-setwebproxy", service, host, port).Run(); err != nil {
		return fmt.Errorf("设置 HTTP 代理失败: %w", err)
	}
	// Set HTTPS proxy
	if err := exec.Command("networksetup", "-setsecurewebproxy", service, host, port).Run(); err != nil {
		return fmt.Errorf("设置 HTTPS 代理失败: %w", err)
	}
	// Set bypass domains for localhost
	if err := exec.Command("networksetup", "-setproxybypassdomains", service,
		"localhost", "127.0.0.1", "*.local", "169.254/16").Run(); err != nil {
		logger.Warn("设置代理旁路失败: %v", err)
	}

	logger.Info("系统代理已设置: %s (服务: %s)", addr, service)
	return nil
}

// ClearSystemProxy disables the macOS system HTTP/HTTPS proxy.
func ClearSystemProxy() error {
	service := getActiveNetworkService()

	exec.Command("networksetup", "-setwebproxystate", service, "off").Run()
	exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run()

	logger.Info("系统代理已清除 (服务: %s)", service)
	return nil
}
