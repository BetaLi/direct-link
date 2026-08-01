package system

import (
	"fmt"
	"syscall"
	"unsafe"

	"directlink/internal/logger"
	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

// SetSystemProxy enables the Windows system HTTP proxy to point to the given address.
func SetSystemProxy(addr string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("设置 ProxyEnable 失败: %w", err)
	}
	if err := key.SetStringValue("ProxyServer", addr); err != nil {
		return fmt.Errorf("设置 ProxyServer 失败: %w", err)
	}
	// Set proxy override for localhost
	if err := key.SetStringValue("ProxyOverride", "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;<local>"); err != nil {
		logger.Warn("设置 ProxyOverride 失败: %v", err)
	}

	notifySettingsChanged()
	logger.Info("系统代理已设置: %s", addr)
	return nil
}

// ClearSystemProxy disables the Windows system HTTP proxy.
func ClearSystemProxy() error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("设置 ProxyEnable 失败: %w", err)
	}

	notifySettingsChanged()
	logger.Info("系统代理已清除")
	return nil
}

// notifySettingsChanged notifies Windows that Internet Settings have changed,
// so browsers pick up the new proxy configuration immediately.
func notifySettingsChanged() {
	dll := syscall.NewLazyDLL("wininet.dll")
	proc := dll.NewProc("InternetSetOptionW")

	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)

	// INTERNET_OPTION_SETTINGS_CHANGED
	r1, _, _ := proc.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	if r1 == 0 {
		logger.Warn("InternetSetOptionW(SETTINGS_CHANGED) 返回 0")
	}

	// INTERNET_OPTION_REFRESH
	r2, _, _ := proc.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)
	if r2 == 0 {
		logger.Warn("InternetSetOptionW(REFRESH) 返回 0")
	}

	// Broadcast WM_SETTINGCHANGE to notify all windows
	broadcastSettingChange()
}

func broadcastSettingChange() {
	dll := syscall.NewLazyDLL("user32.dll")
	proc := dll.NewProc("SendMessageTimeoutW")

	const (
		WM_SETTINGCHANGE = 0x001A
		SMTO_ABORTIFHUNG = 0x0002
	)

	env := uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Environment")))
	ret, _, _ := proc.Call(
		uintptr(0xFFFF), // HWND_BROADCAST
		uintptr(WM_SETTINGCHANGE),
		env,
		uintptr(SMTO_ABORTIFHUNG),
		uintptr(5000),
		uintptr(0),
	)
	_ = ret // result varies by Windows version
}
