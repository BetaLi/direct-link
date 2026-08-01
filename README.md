# DirectLink 加速器 — 技术方案

> 面向客户的桌面直连加速工具，一期支持 Steam 商店与 GitHub

## 1. 方案概述

### 1.1 产品定位

做一个轻量级 Windows 桌面工具，帮助国内用户直连加速访问 Steam 商店/社区和 GitHub。核心能力：

- 一键开关加速，无需配置
- 自动探测最优 IP，定期刷新
- Hosts 模式优先，失效时自动切换到本地代理模式
- 安静后台运行，系统托盘常驻

### 1.2 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| 后端核心 | Go 1.22+ | 编译单文件、网络库丰富、性能好、无运行时依赖 |
| GUI 框架 | Wails v2 | Go + Web 前端，产物为单 exe，界面现代，跨平台可扩展 |
| 前端 | Vue 3 + Tailwind CSS | 轻量，Wails 原生支持，开发快 |
| 打包 | Wails build → NSIS 安装包 | 单 exe 分发，自动安装 |
| 提权 | 嵌入 manifest 请求 admin + UAC | hosts 写入和系统代理设置需要管理员权限 |

## 2. 系统架构

```
┌──────────────────────────────────────────────────┐
│              Wails GUI (Vue 3 + Tailwind)         │
│   ┌──────────┬───────────┬────────────────────┐  │
│   │ 加速开关  │ 状态面板   │ IP日志 / 节点详情   │  │
│   └──────────┴───────────┴────────────────────┘  │
├──────────────────────────────────────────────────┤
│              Go Backend (Core)                     │
│                                                    │
│  ┌──────────────────────────────────────────────┐ │
│  │            探测引擎 Prober                     │ │
│  │  DoH查询(多源) → TCP测速 → TLS握手验证 → 排序  │ │
│  └──────────────────┬───────────────────────────┘ │
│                      │ 最优IP池                    │
│  ┌───────────────────▼───────────────────────────┐ │
│  │          接管控制器 InterceptManager            │ │
│  │                                                │ │
│  │  ┌─────────────┐      ┌────────────────────┐  │ │
│  │  │  HostsMgr   │      │   ProxyServer      │  │ │
│  │  │ hosts读写    │      │ CONNECT隧道代理     │  │ │
│  │  │ 原子替换      │      │ 目标域名→最优IP     │  │ │
│  │  └─────────────┘      └────────────────────┘  │ │
│  │                                                │ │
│  │  ┌──────────────────────────────────────────┐  │ │
│  │  │         自动切换 Switcher                 │  │ │
│  │  │ hosts激活 → 健康检测 → 失效切proxy        │  │ │
│  │  │ proxy激活 → 健康检测 → 恢复切hosts        │  │ │
│  │  └──────────────────────────────────────────┘  │ │
│  └──────────────────────────────────────────────┘ │
│                                                    │
│  ┌──────────────────────────────────────────────┐ │
│  │            系统集成 System                      │ │
│  │  Windows注册表(系统代理) / UAC提权 / 开机自启   │ │
│  └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

## 3. 核心模块设计

### 3.1 探测引擎 (Prober)

负责为每个目标域名找到当前可用的最优 IP。

**流程：**

```
1. DoH 查询：向多个 DoH 服务商查询域名的 A 记录，收集候选 IP
   - Cloudflare: https://1.1.1.1/dns-query?type=A&name=xxx (JSON)
   - Google: https://8.8.8.8/resolve?name=xxx&type=A (JSON)
   - AliDNS: https://223.5.5.5/dns-query?type=A&name=xxx (JSON)
   合并去重，得到候选 IP 池

2. TCP 测速：对每个候选 IP 的 443 端口
   - TCP 连接测试（3秒超时）
   - 测量 RTT（3次取平均）
   - 过滤掉连接失败和 RTT > 500ms 的节点

3. TLS 验证：对通过 TCP 测速的前 N 个 IP
   - 发起 TLS 握手，验证证书域名匹配
   - 验证 SNI 是否被拦截（握手是否成功）
   - 过滤掉 TLS 失败的节点（说明该 IP 无法正常服务该域名）

4. HTTP 可用性：对最优的 1-2 个 IP
   - 发一个 HEAD 请求确认返回正常 HTTP 响应
   - 确认不是被劫持的页面

5. 输出：每个域名 → 最优 IP + 备选 IP 列表 + 探测时间戳
```

**关键 Go 库：**
- `net/http` — DoH 请求、HTTP 探测
- `crypto/tls` — TLS 握手验证
- `net` — TCP 拨号测速
- 并发探测用 goroutine + channel

### 3.2 目标域名配置

一期支持的域名规则表，每条规则包含：域名、DoH 查询名、是否需要 CDN IP 池扩展。

```json
{
  "sites": [
    {
      "name": "Steam 商店",
      "icon": "steam",
      "domains": [
        "store.steampowered.com",
        "steamcommunity.com",
        "api.steampowered.com",
        "steamstatic.com",
        "steamcdn-a.akamaihd.net",
        "community.akamai.steamstatic.com",
        "cdn.akamai.steamstatic.com",
        "cdn.cloudflare.steamstatic.com"
      ]
    },
    {
      "name": "GitHub",
      "icon": "github",
      "domains": [
        "github.com",
        "api.github.com",
        "raw.githubusercontent.com",
        "assets-cdn.github.com",
        "github.global.ssl.fastly.net",
        "codeload.github.com",
        "objects.githubusercontent.com",
        "github.githubassets.com",
        "collector.github.com"
      ]
    }
  ]
}
```

### 3.3 Hosts 管理器 (HostsMgr)

**职责：** 管理系统 hosts 文件中由本工具写入的条目。

**实现要点：**

- **隔离写入**：不直接修改用户原有 hosts 内容，而是维护一个标记块
  ```
  # ===== DirectLink BEGIN =====
  1.2.3.4 store.steampowered.com
  5.6.7.8 github.com
  # ===== DirectLink END =====
  ```
  更新时只替换标记块内的内容，保留用户其余 hosts 配置。

- **原子操作**：先写入临时文件，再 `os.Rename` 替换（Windows 下可能需要先删除再移动），避免写一半断电导致 hosts 损坏。

- **DNS 缓存刷新**：写入 hosts 后调用 `ipconfig /flushdns` 刷新系统 DNS 缓存，使新规则立即生效。

- **退出清理**：程序退出时自动移除标记块，恢复原始 hosts。

**Go 实现：**
```go
const (
    hostsPath = `C:\Windows\System32\drivers\etc\hosts`
    beginMark = "# ===== DirectLink BEGIN ====="
    endMark   = "# ===== DirectLink END ====="
)

// 写入 hosts 条目
func WriteHosts(entries map[string]string) error {
    // 1. 读取现有 hosts
    // 2. 移除旧的 DirectLink 标记块
    // 3. 追加新的标记块 + 条目
    // 4. 原子写入临时文件 → 替换
    // 5. ipconfig /flushdns
}

// 清理 hosts 条目
func CleanHosts() error {
    // 移除标记块，保留其余内容
}
```

### 3.4 本地代理服务器 (ProxyServer)

**职责：** 当 hosts 模式失效时，作为本地 HTTP 代理接管目标域名流量。

**工作原理：**

```
浏览器 → 系统代理(127.0.0.1:8848) → 本地代理服务器
                                        │
                              ┌─────────┴──────────┐
                              │                    │
                        目标域名?              非目标域名?
                              │                    │
                    用最优IP直连              直接转发(不走代理)
                    保留SNI+Host              透明CONNECT隧道
                              │
                       目标服务器(最优IP)
```

**实现要点：**

- 监听 `127.0.0.1:8848`（可配置端口）
- 解析 HTTP 请求：
  - `CONNECT` 方法（HTTPS）：解析目标域名，判断是否在加速列表中
    - 是 → 用最优 IP 拨号 TCP 443，建立双向管道透传
    - 否 → 用域名正常拨号，建立双向管道透传
  - 普通 HTTP 请求：解析 Host 头，同理路由
- **不做 SSL 解密**：只做 TCP 隧道透传，不碰证书，没有中间人问题
- 非加速域名直接 DNS + 直连，不经过 IP 优选，保证透明

**Go 实现（核心）：**

```go
func (p *ProxyServer) handleConnect(conn net.Conn, host string) {
    domain, port := splitHostPort(host)
    
    var target string
    if optimalIP, ok := p.ipPool.Get(domain); ok {
        // 加速域名：用最优 IP
        target = net.JoinHostPort(optimalIP, port)
    } else {
        // 非加速域名：正常 DNS 解析
        target = host
    }
    
    // 拨号到目标
    upstream, err := net.DialTimeout("tcp", target, 10*time.Second)
    if err != nil {
        conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
        conn.Close()
        return
    }
    
    // 回复 200
    conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
    
    // 双向管道
    go io.Copy(upstream, conn)
    io.Copy(conn, upstream)
}
```

### 3.5 自动切换器 (Switcher)

**职责：** 根据网络状况自动在 hosts 模式和代理模式间切换。

**状态机：**

```
                    ┌──────────┐
          启动 ────▶│ HOSTS_ON  │◀──────────────┐
                    └────┬─────┘                │
                         │ 健康检测               │ 检测恢复
                         │ (每60s)               │ (每5min)
                         ▼                      │
                    ┌──────────┐          ┌──────┴─────┐
                    │ HEALTH_FAIL│────────▶│ PROXY_ON    │
                    └──────────┘          └─────────────┘
```

**健康检测逻辑：**

```go
func (s *Switcher) checkHealth(domain string, ip string) bool {
    // 1. TCP 443 连接测试
    conn, err := net.DialTimeout("tcp", ip+":443", 5*time.Second)
    if err != nil {
        return false
    }
    // 2. TLS 握手（验证证书 + SNI）
    tlsConn := tls.Client(conn, &tls.Config{
        ServerName: domain,
    })
    if err := tlsConn.Handshake(); err != nil {
        return false
    }
    // 3. 可选：发 HTTP HEAD 确认不是劫持页面
    return true
}
```

**切换流程：**

- **hosts → proxy**：移除 hosts 标记块 → 启动 ProxyServer → 写入系统代理注册表 → 刷新 DNS 缓存
- **proxy → hosts**：移除系统代理 → 停止 ProxyServer → 写入 hosts 条目 → 刷新 DNS 缓存

### 3.6 系统代理管理 (Windows)

通过修改注册表设置/取消系统代理：

```
HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings

ProxyEnable: REG_DWORD (0/1)
ProxyServer: REG_SZ ("127.0.0.1:8848")
```

写入后发送 `INTERNET_OPTION_SETTINGS_CHANGED` 消息通知系统刷新设置，无需重启浏览器。

**Go 库：** `golang.org/x/sys/windows/registry`

```go
func SetSystemProxy(addr string) error {
    key, _ := registry.OpenKey(registry.CURRENT_USER,
        `Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
        registry.SET_VALUE|registry.SET_VALUE)
    defer key.Close()
    
    key.SetDWordValue("ProxyEnable", 1)
    key.SetStringValue("ProxyServer", addr)
    // 通知系统刷新
    notifySettingsChanged()
}

func ClearSystemProxy() error {
    key, _ := registry.OpenKey(registry.CURRENT_USER,
        `Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
        registry.SET_VALUE)
    defer key.Close()
    
    key.SetDWordValue("ProxyEnable", 0)
    notifySettingsChanged()
}
```

### 3.7 GUI 设计 (Wails + Vue 3)

**主界面布局：**

```
┌─────────────────────────────────────┐
│         DirectLink 加速器            │
│                                     │
│  ┌─────────┐  ┌─────────┐          │
│  │ Steam   │  │ GitHub  │          │
│  │ ● 已加速 │  │ ● 已加速 │          │
│  │ 12ms   │  │ 8ms     │          │
│  └─────────┘  └─────────┘          │
│                                     │
│  当前模式: Hosts 直连                 │
│  最优节点: 1.2.3.4 (Cloudflare)      │
│  下次刷新: 14:32                     │
│                                     │
│  [一键全部加速]  [暂停]  [设置]       │
│                                     │
│  ─── 最近日志 ───                    │
│  14:02 探测 github.com → 140.82.x  │
│  14:02 TLS 验证通过                  │
│  14:02 hosts 已更新 (8条)           │
└─────────────────────────────────────┘
```

**系统托盘：**

- 最小化到托盘，右键菜单：加速/暂停/退出
- 加速状态时图标为绿色，暂停为灰色，异常为红色

**Wails 绑定（Go → 前端暴露的方法）：**

```go
type App struct {
    ctx        context.Context
    prober     *Prober
    intercept  *InterceptManager
}

// Wails 暴露给前端的方法
func (a *App) ToggleSite(siteID string, on bool) error    // 开关某站点加速
func (a *App) ToggleAll(on bool) error                      // 一键全部开关
func (a *App) GetStatus() StatusResponse                    // 获取当前状态
func (a *App) GetLog() []LogEntry                           // 获取日志
func (a *App) Reprobe() error                               // 手动重新探测
func (a *App) GetSettings() Settings                        // 获取设置
func (a *App) SaveSettings(s Settings) error                // 保存设置
```

## 4. 项目目录结构

```
directlink/
├── main.go                      # Wails 入口
├── wails.json                   # Wails 配置
├── app.go                       # App 结构体，Wails 绑定方法
├── go.mod
├── go.sum
│
├── internal/
│   ├── prober/
│   │   ├── prober.go            # 探测引擎主逻辑
│   │   ├── doh.go               # DoH 查询
│   │   ├── speedtest.go         # TCP 测速
│   │   └── tlscheck.go          # TLS 握手验证
│   │
│   ├── intercept/
│   │   ├── manager.go           # 接管控制器（统一管理 hosts + proxy）
│   │   ├── hosts.go             # Hosts 文件读写
│   │   ├── proxy.go             # 本地代理服务器
│   │   └── switcher.go          # 自动切换逻辑
│   │
│   ├── system/
│   │   ├── proxy_windows.go    # Windows 系统代理设置
│   │   ├── admin_windows.go    # UAC 提权
│   │   ├── autostart_windows.go # 开机自启
│   │   └── tray.go             # 系统托盘
│   │
│   ├── config/
│   │   ├── config.go            # 配置加载/保存
│   │   └── rules.go             # 域名规则表（内置 + 用户自定义）
│   │
│   └── logger/
│       └── logger.go            # 日志记录
│
├── frontend/
│   ├── src/
│   │   ├── App.vue              # 主界面
│   │   ├── components/
│   │   │   ├── SiteCard.vue     # 站点加速卡片
│   │   │   ├── StatusPanel.vue  # 状态面板
│   │   │   └── LogView.vue     # 日志视图
│   │   ├── stores/
│   │   │   └── app.ts           # Pinia 状态管理
│   │   └── main.ts
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
│
├── build/
│   ├── windows/
│   │   ├── icon.ico             # 应用图标
│   │   └── info.json            # NSIS 安装包配置
│   └── wails.exe.manifest       # 请求 admin 权限
│
└── assets/
    └── rules.json               # 内置域名规则表
```

## 5. 关键技术难点与解决

### 5.1 DoH 查询被污染

**问题：** 如果系统 DNS 已被污染，DoH 请求本身可能到达不了正确的 DoH 服务器。

**解决：**
- DoH 请求直接用 IP 访问（`https://1.1.1.1/dns-query`），不依赖 DNS 解析
- 多源查询：Cloudflare、Google、AliDNS 三个源都查，取并集
- 如果某个 DoH 源也连不上，跳过它，用其他源的结果

### 5.2 TLS 透传的 SNI 问题

**问题：** 代理模式下，用最优 IP 连接目标服务器时，TLS 握手需要发送正确的 SNI，否则服务器可能拒绝连接或返回错误证书。

**解决：**
- CONNECT 隧道是 TCP 透传，客户端的 TLS ClientHello（含 SNI）原封不动到达目标服务器
- 代理只负责用最优 IP 建立 TCP 连接，之后双向管道透传，SNI 由浏览器自然发送
- 不需要代理自己处理 TLS，所以不存在 SNI 伪造问题

### 5.3 管理员权限

**问题：** 修改 hosts 和（某些情况下）系统代理需要管理员权限。

**解决：**
- 在 exe 的 manifest 中声明 `requireAdministrator`，程序启动即弹 UAC
- 或者在运行时检测权限，需要提权时用 `ShellExecuteW` + `"runas"` 重新启动自身
- 后台运行时不需要持续弹窗（Windows 记住 UAC 决策）

### 5.4 hosts 模式下证书错误

**问题：** 某些 CDN IP 可能没有为该域名配置证书，TLS 握手会因证书不匹配而失败。

**解决：**
- 探测阶段的 TLS 验证步骤会过滤掉这类 IP，只保留证书验证通过的
- 如果所有 IP 都证书验证失败，自动切换到代理模式（代理模式下证书由浏览器验证，代理不参与）
- 极端情况下，提示用户该域名暂时无法加速

### 5.5 端口冲突

**问题：** 本地代理端口 8848 可能被占用。

**解决：**
- 启动时检测端口占用，自动选一个空闲端口
- 系统代理设置使用实际端口

## 6. 配置文件设计

```json
{
  "version": "1.0.0",
  "sites": {
    "steam": { "enabled": true },
    "github": { "enabled": true }
  },
  "advanced": {
    "proxyPort": 8848,
    "probeInterval": 1800,
    "healthCheckInterval": 60,
    "dohProviders": ["cloudflare", "google", "alidns"],
    "maxIPsPerDomain": 5,
    "preferredMode": "auto"
  },
  "customSites": [],
  "autostart": true,
  "minimizeToTray": true
}
```

配置文件存储在 `%APPDATA%/DirectLink/config.json`。

## 7. 构建与打包

### 7.1 开发环境

```bash
# 安装 Go
winget install GoLang.Go

# 安装 Node.js（前端编译用，用户不需要）
winget install OpenJS.NodeJS.LTS

# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 初始化项目
wails init -n directlink -t vue
```

### 7.2 构建

```bash
# 开发模式（热重载）
wails dev

# 构建 exe
wails build -platform windows/amd64

# 产物在 build/bin/directlink.exe
```

### 7.3 NSIS 安装包

Wails build 默认生成单 exe。如需 NSIS 安装包：

```bash
# 用 makensis 打包
# 或用 electron-builder 的 NSIS 功能（需要 Node 环境）
# 最简单：直接分发 exe，首次运行时自配置
```

考虑到用户系统没有 Node.js，建议用 Go 生态的 NSIS 打包方案：
- 用 `github.com/iamacarpet/go-win64
        
    
        nsis` 或直接调用 NSIS 编译器

## 8. 一期开发计划

### Phase 1 — 核心引擎（1-2周）

- [ ] 探测引擎：DoH 查询 + TCP 测速 + TLS 验证
- [ ] 域名规则表：Steam + GitHub 域名配置
- [ ] Hosts 管理器：标记块读写 + DNS 刷新
- [ ] 基础 CLI 验证（命令行测试探测和 hosts 写入）

### Phase 2 — 代理与切换（1-2周）

- [ ] 本地代理服务器：CONNECT 隧道 + 最优 IP 路由
- [ ] 系统代理设置：注册表写入 + 通知刷新
- [ ] 自动切换器：健康检测 + 模式切换状态机
- [ ] 配置文件加载/保存

### Phase 3 — GUI 与打包（1-2周）

- [ ] Wails 项目搭建 + Vue 前端
- [ ] 主界面：站点卡片 + 状态面板 + 日志
- [ ] 系统托盘：最小化 + 右键菜单
- [ ] Admin manifest 提权
- [ ] 构建 exe + 打包测试

### Phase 4 — 完善与测试（1周）

- [ ] 多种网络环境测试（电信/联通/移动）
- [ ] 异常恢复（崩溃后 hosts 清理）
- [ ] 日志文件持久化
- [ ] 开机自启

## 9. 风险评估

| 风险 | 影响 | 应对 |
|---|---|---|
| 部分 CDN IP 证书不匹配 | hosts 模式不可用 | TLS 验证过滤 + 自动切代理模式 |
| DoH 请求本身被拦截 | 探测拿不到干净 IP | 多 DoH 源 + IP 直连 DoH |
| SNI 审查（非 DNS 污染） | hosts 和代理都可能失效 | 代理模式下可扩展 SNI 伪装（二期） |
| 管理员权限被拒 | 无法修改 hosts/系统代理 | 检测权限，提示用户；降级为仅代理模式 |
| IP 快速失效 | 加速效果不稳定 | 缩短探测周期，增加健康检测频率 |
```
