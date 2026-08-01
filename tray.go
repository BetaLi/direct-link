package main

import (
	_ "embed"

	"github.com/getlantern/systray"
	"directlink/internal/logger"
	"directlink/internal/system"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/tray_idle.ico
var trayIconIdle []byte

//go:embed build/windows/tray_on.ico
var trayIconOn []byte

//go:embed build/windows/tray_off.ico
var trayIconOff []byte

// Tray manages the system tray icon and menu.
type Tray struct {
	app        *App
	mShow      *systray.MenuItem
	mToggle    *systray.MenuItem
	mAutostart *systray.MenuItem
	mQuit      *systray.MenuItem
}

// NewTray creates a new Tray instance.
func NewTray(app *App) *Tray {
	return &Tray{app: app}
}

// Start initializes the system tray. Must run on the main thread on macOS.
// On Windows, it can run in a goroutine.
func (t *Tray) Start() {
	systray.Run(t.onReady, t.onExit)
}

func (t *Tray) onReady() {
	systray.SetIcon(trayIconIdle)
	systray.SetTitle("DirectLink")
	systray.SetTooltip("DirectLink 加速器 - 未运行")

	t.mShow = systray.AddMenuItem("显示主窗口", "显示主窗口")
	systray.AddSeparator()
	t.mToggle = systray.AddMenuItem("开启加速", "启动/停止加速")
	t.mAutostart = systray.AddMenuItemCheckbox("开机自启", "开机自动启动", system.IsAutoStartEnabled())
	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("退出", "退出程序")

	go t.loop()
}

func (t *Tray) onExit() {
	// Cleanup is handled in App.Shutdown
	logger.Info("系统托盘已退出")
}

// loop handles tray menu clicks.
func (t *Tray) loop() {
	for {
		select {
		case <-t.mShow.ClickedCh:
			if t.app.ctx != nil {
				wailsRuntime.WindowShow(t.app.ctx)
			}

		case <-t.mToggle.ClickedCh:
			if t.app.mgr != nil && t.app.mgr.IsRunning() {
				_ = t.app.Stop()
			} else {
				_ = t.app.Start()
			}

		case <-t.mAutostart.ClickedCh:
			if t.mAutostart.Checked() {
				system.RemoveAutoStart()
				t.mAutostart.Uncheck()
				logger.Info("已关闭开机自启")
			} else {
				exePath, err := system.GetExePath()
				if err == nil {
					system.SetAutoStart(exePath)
					t.mAutostart.Check()
					logger.Info("已开启开机自启: %s", exePath)
				}
			}

		case <-t.mQuit.ClickedCh:
			if t.app.mgr != nil && t.app.mgr.IsRunning() {
				t.app.mgr.Stop()
			}
			systray.Quit()
			return
		}
	}
}

// UpdateIcon updates the tray icon based on running state.
func (t *Tray) UpdateIcon(running bool, mode string) {
	if running {
		systray.SetIcon(trayIconOn)
		systray.SetTooltip("DirectLink 加速器 - 运行中 (" + mode + ")")
		t.mToggle.SetTitle("停止加速")
	} else {
		systray.SetIcon(trayIconIdle)
		systray.SetTooltip("DirectLink 加速器 - 未运行")
		t.mToggle.SetTitle("开启加速")
	}
}
