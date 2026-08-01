package intercept

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"directlink/internal/logger"
)

const (
	beginMark = "# ===== DirectLink BEGIN ====="
	endMark   = "# ===== DirectLink END ====="
)

// hostsPath returns the system hosts file path.
func hostsPath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

// HostsMgr manages the DirectLink section in the hosts file.
type HostsMgr struct {
	mu sync.Mutex
}

func NewHostsMgr() *HostsMgr {
	return &HostsMgr{}
}

// WriteEntries writes domain→IP mappings into the hosts file within the DirectLink block.
func (h *HostsMgr) WriteEntries(entries map[string]string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hostsFile := hostsPath()

	// Read current hosts content
	content, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("读取 hosts 文件失败: %w", err)
	}

	// Remove old DirectLink block, keep the rest
	cleanContent := h.removeBlock(string(content))

	// Build new block
	var buf bytes.Buffer
	buf.WriteString(strings.TrimRight(cleanContent, "\r\n"))
	buf.WriteString("\r\n\r\n")
	buf.WriteString(beginMark)
	buf.WriteString("\r\n")
	for domain, ip := range entries {
		buf.WriteString(fmt.Sprintf("%s %s\r\n", ip, domain))
	}
	buf.WriteString(endMark)
	buf.WriteString("\r\n")

	// Atomic write: write to temp file, then replace
	tmpFile := hostsFile + ".directlink.tmp"
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// On Windows, need to remove original before rename
	os.Remove(hostsFile)
	if err := os.Rename(tmpFile, hostsFile); err != nil {
		return fmt.Errorf("替换 hosts 文件失败: %w", err)
	}

	// Flush DNS cache
	h.flushDNS()

	logger.Info("hosts 已更新，写入 %d 条记录", len(entries))
	return nil
}

// Clean removes the DirectLink block from the hosts file.
func (h *HostsMgr) Clean() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hostsFile := hostsPath()

	content, err := os.ReadFile(hostsFile)
	if err != nil {
		// If hosts file doesn't exist, nothing to clean
		return nil
	}

	cleanContent := h.removeBlock(string(content))

	// Only write if content changed
	if cleanContent == string(content) {
		return nil
	}

	tmpFile := hostsFile + ".directlink.tmp"
	if err := os.WriteFile(tmpFile, []byte(cleanContent), 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	os.Remove(hostsFile)
	if err := os.Rename(tmpFile, hostsFile); err != nil {
		return fmt.Errorf("替换 hosts 文件失败: %w", err)
	}

	h.flushDNS()
	logger.Info("hosts 已清理，恢复原始状态")
	return nil
}

// removeBlock removes the DirectLink block from the content.
func (h *HostsMgr) removeBlock(content string) string {
	lines := strings.Split(content, "\n")

	var result []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == beginMark {
			inBlock = true
			continue
		}
		if trimmed == endMark {
			inBlock = false
			continue
		}
		if inBlock {
			continue
		}
		result = append(result, line)
	}

	// Trim trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n") + "\n"
}

// flushDNS flushes the system DNS cache.
func (h *HostsMgr) flushDNS() {
	if runtime.GOOS != "windows" {
		return
	}
	cmd := exec.Command("ipconfig", "/flushdns")
	if err := cmd.Run(); err != nil {
		logger.Warn("刷新 DNS 缓存失败: %v", err)
	} else {
		logger.Debug("DNS 缓存已刷新")
	}
}
