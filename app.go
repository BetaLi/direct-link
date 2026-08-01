package main

import (
	"context"
	"fmt"

	"directlink/internal/config"
	"directlink/internal/intercept"
	"directlink/internal/logger"
	"directlink/internal/system"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct with methods exposed to the Wails frontend.
type App struct {
	ctx   context.Context
	mgr   *intercept.Manager
	cfg   *config.AppConfig
	tray  *Tray
}

// NewApp creates a new App instance.
func NewApp() *App {
	logger.Init("")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("加载配置失败: %v", err)
		cfg = config.DefaultConfig()
	}

	app := &App{cfg: cfg}
	app.tray = NewTray(app)
	return app
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	logger.Info("DirectLink GUI 启动")

	// Start system tray in a goroutine
	go a.tray.Start()
}

// DomReady is called when the frontend DOM is ready.
func (a *App) DomReady() {
	// Push initial status
	a.emitStatus()
}

// Shutdown is called when the app is actually quitting (from tray quit).
func (a *App) Shutdown() {
	if a.mgr != nil && a.mgr.IsRunning() {
		a.mgr.Stop()
		logger.Info("DirectLink 已在退出时停止加速")
	}
}

// HideToTray hides the window and shows a notification.
func (a *App) HideToTray() {
	if a.ctx != nil {
		wailsRuntime.WindowHide(a.ctx)
	}
	logger.Info("窗口已最小化到系统托盘")
}

// ShowWindow shows the main window.
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowShow(a.ctx)
	}
}

// Quit closes the application completely (called from frontend quit button).
func (a *App) Quit() {
	if a.mgr != nil && a.mgr.IsRunning() {
		a.mgr.Stop()
	}
	wailsRuntime.Quit(a.ctx)
}

// GetStatus returns the current acceleration status.
func (a *App) GetStatus() intercept.Status {
	if a.mgr == nil {
		return intercept.Status{
			Running: false,
			Mode:    "off",
			Sites:   make(map[string]intercept.SiteStatus),
		}
	}
	return a.mgr.GetStatus()
}

// GetLog returns recent log entries.
func (a *App) GetLog() []logger.LogEntry {
	return logger.Get().GetEntries()
}

// GetConfig returns the current configuration.
func (a *App) GetConfig() *config.AppConfig {
	return a.cfg
}

// SaveConfig saves the configuration.
func (a *App) SaveConfig(cfg *config.AppConfig) error {
	a.cfg = cfg
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	if a.mgr != nil {
		a.mgr.UpdateConfig(cfg)
	}
	return nil
}

// ToggleSite enables or disables a site.
func (a *App) ToggleSite(siteID string, enabled bool) error {
	if a.cfg.Sites == nil {
		a.cfg.Sites = make(map[string]config.SiteConfig)
	}
	a.cfg.Sites[siteID] = config.SiteConfig{Enabled: enabled}
	if err := config.Save(a.cfg); err != nil {
		return err
	}
	if a.mgr != nil {
		a.mgr.UpdateConfig(a.cfg)
	}
	a.emitStatus()
	return nil
}

// Start begins acceleration.
func (a *App) Start() error {
	if a.mgr != nil && a.mgr.IsRunning() {
		return fmt.Errorf("加速已在运行中")
	}

	a.mgr = intercept.NewManager(a.cfg)
	if err := a.mgr.Start(); err != nil {
		return err
	}

	a.emitStatus()
	return nil
}

// Stop stops acceleration.
func (a *App) Stop() error {
	if a.mgr == nil || !a.mgr.IsRunning() {
		return fmt.Errorf("加速未运行")
	}
	a.mgr.Stop()
	a.emitStatus()
	return nil
}

// Reprobe forces an immediate re-probe.
func (a *App) Reprobe() error {
	if a.mgr == nil || !a.mgr.IsRunning() {
		return fmt.Errorf("加速未运行")
	}
	return a.mgr.Reprobe()
}

// SetAutoStart enables or disables autostart.
func (a *App) SetAutoStart(enabled bool) error {
	if enabled {
		exePath, err := system.GetExePath()
		if err != nil {
			return fmt.Errorf("获取可执行文件路径失败: %w", err)
		}
		if err := system.SetAutoStart(exePath); err != nil {
			return fmt.Errorf("设置开机自启失败: %w", err)
		}
		logger.Info("已开启开机自启")
	} else {
		if err := system.RemoveAutoStart(); err != nil {
			return fmt.Errorf("关闭开机自启失败: %w", err)
		}
		logger.Info("已关闭开机自启")
	}
	return nil
}

// GetAutoStart returns whether autostart is enabled.
func (a *App) GetAutoStart() bool {
	return system.IsAutoStartEnabled()
}

// Minimize minimizes the window (called from frontend).
func (a *App) Minimize() {
	wailsRuntime.WindowMinimise(a.ctx)
}

// emitStatus sends the current status to both the tray and frontend.
func (a *App) emitStatus() {
	status := a.GetStatus()

	// Update tray
	go func() {
		if a.tray != nil {
			a.tray.UpdateIcon(status.Running, status.Mode)
		}
	}()

	// Push to frontend via event
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "status:update", status)
	}
}
