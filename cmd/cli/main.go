//go:build !wails

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"directlink/internal/config"
	"directlink/internal/intercept"
	"directlink/internal/logger"
	"directlink/internal/prober"
	"directlink/internal/system"
)

func main() {
	logger.Init("")

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "probe":
			cmdProbe(os.Args[2:])
			return
		case "status":
			cmdStatus()
			return
		case "clean":
			cmdClean()
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	cmdRun(nil)
}

func printUsage() {
	fmt.Println(`DirectLink — Steam & GitHub 直连加速器

Usage:
  directlink [command]

Commands:
  run       启动加速器 (默认)
  probe     探测最优 IP 并显示结果
  status    查看当前状态
  clean     清理 hosts 并恢复系统代理
  help      显示帮助信息`)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	site := fs.String("site", "", "只加速指定站点 (steam/github)")
	fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("加载配置失败: %v", err)
		os.Exit(1)
	}

	if *site != "" {
		for k := range cfg.Sites {
			if k != *site {
				cfg.Sites[k] = config.SiteConfig{Enabled: false}
			}
		}
	}

	mgr := intercept.NewManager(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n正在停止...")
		mgr.Stop()
		os.Exit(0)
	}()

	fmt.Println("DirectLink 加速器启动中...")
	if err := mgr.Start(); err != nil {
		logger.Error("启动失败: %v", err)
		fmt.Printf("启动失败: %v\n", err)
		os.Exit(1)
	}

	status := mgr.GetStatus()
	fmt.Printf("\n✓ 加速已启动\n")
	fmt.Printf("  模式: %s\n", status.Mode)
	for _, s := range status.Sites {
		if s.Enabled {
			fmt.Printf("  %s: %s (%dms, %d/%d 域名已连接)\n",
				s.Name, s.BestIP, s.Latency, s.Connected, s.Domains)
		}
	}
	fmt.Println("\n按 Ctrl+C 停止...")

	select {}
}

func cmdProbe(args []string) {
	cfg, err := config.Load()
	if err != nil {
		logger.Error("加载配置失败: %v", err)
		os.Exit(1)
	}

	domains := config.GetEnabledDomains(cfg)
	if len(domains) == 0 {
		fmt.Println("没有启用的站点")
		os.Exit(1)
	}

	domainList := make([]string, len(domains))
	for i, d := range domains {
		domainList[i] = d.Domain
	}

	fmt.Printf("并发探测 %d 个域名...\n\n", len(domainList))

	p := prober.New(cfg.Advanced.MaxIPsPerDomain, cfg.Advanced.DohProviders)
	results := p.ProbeDomains(domainList)

	rules := config.BuiltinRules()
	for _, rule := range rules {
		if siteCfg, ok := cfg.Sites[rule.ID]; !ok || !siteCfg.Enabled {
			continue
		}
		fmt.Printf("【%s】\n", rule.Name)
		for _, dr := range rule.Domains {
			result, ok := results[dr.Domain]
			if !ok {
				fmt.Printf("  ✗ %-35s 探测失败\n", dr.Domain)
				continue
			}
			if result.BestIP == "" {
				fmt.Printf("  ✗ %-35s 无可用 IP\n", dr.Domain)
				continue
			}
			fmt.Printf("  ✓ %-35s → %-15s  (%dms)\n", dr.Domain, result.BestIP, result.Latency)
			if len(result.BackupIPs) > 0 {
				fmt.Printf("    备选: %v\n", result.BackupIPs)
			}
		}
		fmt.Println()
	}
}

func cmdStatus() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	p := prober.New(cfg.Advanced.MaxIPsPerDomain, cfg.Advanced.DohProviders)
	results := p.GetAllResults()

	fmt.Println("DirectLink 状态:")
	fmt.Printf("  配置版本: %s\n", cfg.Version)
	for siteID, siteCfg := range cfg.Sites {
		fmt.Printf("  %s: %v\n", siteID, siteCfg.Enabled)
	}
	fmt.Printf("  代理端口: %d\n", cfg.Advanced.ProxyPort)
	fmt.Printf("  探测周期: %ds\n", cfg.Advanced.ProbeInterval)
	fmt.Printf("  健康检测: %ds\n", cfg.Advanced.HealthCheckInterval)
	fmt.Printf("  DoH 源: %v\n", cfg.Advanced.DohProviders)
	fmt.Printf("  已探测域名: %d\n", len(results))
}

func cmdClean() {
	hostsMgr := intercept.NewHostsMgr()
	if err := hostsMgr.Clean(); err != nil {
		fmt.Printf("清理 hosts 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ hosts 已清理")

	_ = system.ClearSystemProxy
	fmt.Println("✓ 系统代理已清除")
}
