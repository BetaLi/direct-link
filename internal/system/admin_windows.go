package system

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// AppDataDir returns the DirectLink AppData directory.
func AppDataDir() string {
	dir := filepath.Join(os.Getenv("APPDATA"), "DirectLink")
	os.MkdirAll(dir, 0755)
	return dir
}

// SetAutoStart adds the application to Windows startup.
func SetAutoStart(exePath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue("DirectLink", exePath)
}

// RemoveAutoStart removes the application from Windows startup.
func RemoveAutoStart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.DeleteValue("DirectLink")
}

// IsAutoStartEnabled checks if autostart is enabled and points to current exe.
func IsAutoStartEnabled() bool {
	exePath, err := GetExePath()
	if err != nil {
		return false
	}
	val, err := ReadAutoStartValue()
	if err != nil {
		return false
	}
	return strings.EqualFold(val, exePath)
}

// ReadAutoStartValue reads the autostart registry value.
func ReadAutoStartValue() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	val, _, err := key.GetStringValue("DirectLink")
	return val, err
}

// GetExePath returns the path to the current executable.
func GetExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe = strings.ReplaceAll(exe, "/", "\\")
	return exe, nil
}
