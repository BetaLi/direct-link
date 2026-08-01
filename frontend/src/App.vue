<template>
  <div class="root">
    <!-- Header -->
    <header class="header">
      <div class="header-left">
        <div class="logo" :class="isRunning ? 'logo-on' : 'logo-idle'">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
        </div>
        <div>
          <div class="logo-text">DirectLink</div>
          <div class="logo-sub">直连加速器</div>
        </div>
      </div>
      <div class="header-right">
        <button @click="toggleSettings" class="icon-btn" title="设置">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        </button>
        <button @click="minimize" class="icon-btn" title="最小化到托盘">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19.5 8.25l-7.5 7.5-7.5-7.5"/></svg>
        </button>
      </div>
    </header>

    <!-- Status Section -->
    <div class="status-section">
      <div class="status-ring" :class="isRunning ? 'ring-on' : 'ring-off'">
        <div class="status-inner" :class="isRunning ? 'inner-on' : 'inner-off'">
          <div v-if="isRunning" class="status-dot dot-on"></div>
          <div v-else class="status-dot dot-off"></div>
        </div>
      </div>
      <div class="status-text">{{ statusText }}</div>
      <div v-if="isRunning" class="status-info">
        {{ modeLabel }} · {{ totalConnected }}/{{ totalDomains }} 域名
      </div>
      <button @click="toggleAccelerate" :disabled="isLoading" class="main-btn"
        :class="isRunning ? 'btn-stop' : 'btn-start'">
        <span v-if="isLoading" class="spin spinner"></span>
        {{ isLoading ? '处理中...' : (isRunning ? '停止加速' : '一键加速') }}
      </button>
    </div>

    <!-- Site Cards -->
    <div class="sites-scroll">
      <div v-for="site in sites" :key="site.id"
        class="site-card slide-in"
        :class="site.enabled && isRunning && site.bestIP ? 'card-active' : 'card-normal'">
        <div class="site-row">
          <div class="site-info">
            <div class="site-icon" :class="site.enabled ? 'icon-on' : 'icon-off'">{{ siteIcon(site.id) }}</div>
            <div>
              <div class="site-name">{{ site.name }}</div>
              <div class="site-meta">
                {{ site.connected }}/{{ site.domains }}
                <span v-if="site.latency > 0" :class="latencyColor(site.latency)"> · {{ site.latency }}ms</span>
              </div>
            </div>
          </div>
          <button @click="toggleSite(site.id)" class="toggle-switch"
            :class="site.enabled ? 'switch-on' : 'switch-off'">
            <span class="knob" :class="site.enabled ? 'knob-on' : 'knob-off'"></span>
          </button>
        </div>
        <div v-if="site.enabled && site.bestIP && isRunning" class="ip-box">
          <span class="ip-label">最优 IP</span>
          <span class="ip-value">{{ site.bestIP }}</span>
        </div>
        <div v-else-if="site.enabled && !site.bestIP && isRunning" class="ip-warn">
          部分域名被封锁，TCP 无法连接
        </div>
      </div>
    </div>

    <!-- Log -->
    <div class="log-panel">
      <button @click="showLog = !showLog" class="log-header">
        <span class="log-title">运行日志</span>
        <span class="log-count">{{ logEntries.length }}</span>
      </button>
      <div v-show="showLog" class="log-list">
        <div v-for="(e,i) in logEntries.slice(0,30)" :key="i" class="log-line">
          <span class="log-time">{{ formatTime(e.time) }}</span>
          <span :class="'log-level-' + e.level.toLowerCase()">{{ e.level }}</span>
          <span class="log-msg">{{ e.msg }}</span>
        </div>
        <div v-if="logEntries.length===0" class="log-empty">暂无日志</div>
      </div>
    </div>

    <!-- Footer -->
    <footer class="footer">
      <button @click="reprobe" :disabled="!isRunning" class="footer-btn">重新探测</button>
      <span class="version">v1.0.0</span>
    </footer>

    <!-- Settings Modal -->
    <transition name="fade">
      <div v-if="showSettings" @click.self="showSettings=false" class="modal-overlay">
        <div class="modal-card slide-in">
          <div class="modal-header">
            <span class="modal-title">设置</span>
            <button @click="showSettings=false" class="icon-btn">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>
          </div>
          <div class="modal-body">
            <div class="setting-row">
              <div>
                <div class="setting-label">开机自启</div>
                <div class="setting-desc">Windows 启动时自动运行</div>
              </div>
              <button @click="toggleAutostart" class="toggle-switch"
                :class="autoStart ? 'switch-on' : 'switch-off'">
                <span class="knob" :class="autoStart ? 'knob-on' : 'knob-off'"></span>
              </button>
            </div>
            <button @click="cleanHosts" class="setting-action">清理 hosts 文件</button>
            <button @click="quit" class="setting-quit">退出程序</button>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, computed, onMounted, onUnmounted } from 'vue'
import { GetStatus, GetLog, Start, Stop, ToggleSite, Reprobe, SetAutoStart, GetAutoStart, Minimize, Quit, CleanHosts } from './wailsjs/go/main/App'
import { EventsOn } from './wailsjs/runtime/runtime'

interface SiteStatus { id:string; name:string; icon:string; enabled:boolean; bestIP:string; latency:number; domains:number; connected:number }
interface Status { running:boolean; mode:string; sites:Record<string,Omit<SiteStatus,'id'>> }
interface LogEntry { time:string; level:string; msg:string }

export default defineComponent({
  name: 'App',
  setup() {
    const isRunning = ref(false)
    const isLoading = ref(false)
    const currentMode = ref('off')
    const sites = ref<SiteStatus[]>([])
    const logEntries = ref<LogEntry[]>([])
    const showLog = ref(false)
    const showSettings = ref(false)
    const autoStart = ref(false)
    let pollTimer: number | null = null

    async function refreshStatus() {
      try {
        const s = await GetStatus() as unknown as Status
        if (!s) return
        isRunning.value = s.running; currentMode.value = s.mode
        const list: SiteStatus[] = []
        for (const [id, v] of Object.entries(s.sites||{})) list.push({...v, id})
        sites.value = list
      } catch {}
    }
    async function refreshLog() { try { logEntries.value = (await GetLog() as unknown as LogEntry[]) || [] } catch {} }
    async function toggleAccelerate() {
      isLoading.value = true
      try { if (isRunning.value) await Stop(); else await Start() } catch {}
      setTimeout(() => { refreshStatus(); refreshLog(); isLoading.value = false }, 800)
    }
    async function toggleSite(id:string) {
      const s = sites.value.find(x=>x.id===id); if(!s) return
      await ToggleSite(id, !s.enabled); await refreshStatus()
    }
    async function reprobe() { try { await Reprobe() } catch {}; setTimeout(refreshStatus, 2000) }
    async function toggleAutostart() { autoStart.value = !autoStart.value; try { await SetAutoStart(autoStart.value) } catch {} }
    async function minimize() { try { await Minimize() } catch {} }
    async function quit() { try { await Quit() } catch {} }
    async function toggleSettings() {
      showSettings.value = !showSettings.value
      if (showSettings.value) { try { autoStart.value = await GetAutoStart() } catch {} }
    }
    async function cleanHosts() { try { await CleanHosts() } catch {} }

    function siteIcon(id:string) { return id==='steam'?'S':id==='github'?'G':'?' }
    function formatTime(t:string) { try { return new Date(t).toLocaleTimeString('zh-CN',{hour12:false}) } catch { return '' } }
    function latencyColor(ms:number) { return ms<100?'latency-good':ms<300?'latency-mid':'latency-bad' }

    const totalConnected = computed(() => sites.value.reduce((s,x)=>s+x.connected,0))
    const totalDomains = computed(() => sites.value.reduce((s,x)=>s+x.domains,0))
    const modeLabel = computed(() => currentMode.value==='hosts'?'Hosts 直连':currentMode.value==='proxy'?'代理模式':'')
    const statusText = computed(() => isRunning.value ? '加速运行中' : '加速器已停止')

    onMounted(() => {
      refreshStatus(); refreshLog()
      pollTimer = window.setInterval(() => { refreshStatus(); refreshLog() }, 3000)
      try { EventsOn('status:update', () => { refreshStatus(); refreshLog() }) } catch {}
    })
    onUnmounted(() => { if(pollTimer) clearInterval(pollTimer) })

    return {
      isRunning, isLoading, sites, logEntries, showLog, showSettings, autoStart,
      toggleAccelerate, toggleSite, reprobe, toggleAutostart, minimize, quit, toggleSettings, cleanHosts,
      siteIcon, formatTime, latencyColor,
      totalConnected, totalDomains, modeLabel, statusText,
    }
  }
})
</script>

<style scoped>
.root { display:flex; flex-direction:column; height:100vh; background:#161618; color:#e4e4e7; }

/* Header */
.header { display:flex; align-items:center; justify-content:space-between; padding:12px 16px; border-bottom:1px solid #2e2e33; }
.header-left { display:flex; align-items:center; gap:10px; }
.logo { width:32px; height:32px; border-radius:8px; display:flex; align-items:center; justify-content:center; }
.logo-on { background:rgba(34,197,94,.15); color:#22c55e; }
.logo-idle { background:rgba(59,130,246,.15); color:#3b82f6; }
.logo-text { font-size:14px; font-weight:700; }
.logo-sub { font-size:10px; color:#71717a; }
.header-right { display:flex; gap:4px; }
.icon-btn { padding:6px; border-radius:8px; color:#71717a; background:none; border:none; cursor:pointer; transition:all .15s; }
.icon-btn:hover { color:#e4e4e7; background:rgba(63,63,70,.3); }

/* Status */
.status-section { display:flex; flex-direction:column; align-items:center; padding:16px; gap:8px; }
.status-ring { width:64px; height:64px; border-radius:50%; display:flex; align-items:center; justify-content:center; }
.ring-on { background:rgba(34,197,94,.1); }
.ring-off { background:rgba(63,63,70,.15); }
.status-inner { width:40px; height:40px; border-radius:50%; display:flex; align-items:center; justify-content:center; }
.inner-on { background:rgba(34,197,94,.2); }
.inner-off { background:rgba(63,63,70,.2); }
.status-dot { width:12px; height:12px; border-radius:50%; }
.dot-on { background:#22c55e; }
.dot-off { background:#71717a; }
.status-text { font-size:12px; color:#71717a; }
.status-info { font-size:10px; color:#71717a; }

.main-btn { width:100%; padding:10px; border-radius:12px; font-size:13px; font-weight:500; border:1px solid transparent; cursor:pointer; transition:all .2s; }
.btn-start { background:rgba(34,197,94,.15); color:#22c55e; border-color:rgba(34,197,94,.3); }
.btn-start:hover { background:rgba(34,197,94,.25); }
.btn-stop { background:rgba(239,68,68,.15); color:#ef4444; border-color:rgba(239,68,68,.3); }
.btn-stop:hover { background:rgba(239,68,68,.25); }
.main-btn:disabled { opacity:.5; cursor:wait; }

/* Sites */
.sites-scroll { flex:1; overflow-y:auto; padding:0 16px; display:flex; flex-direction:column; gap:10px; }
.site-card { background:#232328; border-radius:12px; padding:12px; }
.card-active { border:1px solid rgba(34,197,94,.3); }
.card-normal { border:1px solid #2e2e33; }
.site-row { display:flex; align-items:center; justify-content:space-between; margin-bottom:8px; }
.site-info { display:flex; align-items:center; gap:10px; }
.site-icon { width:36px; height:36px; border-radius:8px; display:flex; align-items:center; justify-content:center; font-size:13px; font-weight:700; }
.icon-on { background:rgba(59,130,246,.15); color:#3b82f6; }
.icon-off { background:rgba(63,63,70,.15); color:#71717a; }
.site-name { font-size:13px; font-weight:600; }
.site-meta { font-size:10px; color:#71717a; }
.latency-good { color:#22c55e; }
.latency-mid { color:#eab308; }
.latency-bad { color:#ef4444; }

.ip-box { display:flex; align-items:center; justify-content:space-between; background:rgba(0,0,0,.2); border-radius:8px; padding:6px 10px; font-size:11px; }
.ip-label { color:#71717a; }
.ip-value { font-family:monospace; color:#22c55e; }
.ip-warn { font-size:11px; color:rgba(239,68,68,.6); }

/* Toggle */
.toggle-switch { width:40px; height:22px; border-radius:11px; border:none; cursor:pointer; position:relative; display:flex; align-items:center; padding:0; }
.switch-on { background:rgba(34,197,94,.8); }
.switch-off { background:#52525b; }
.knob { width:16px; height:16px; background:#fff; border-radius:50%; position:absolute; transition:transform .2s cubic-bezier(.4,0,.2,1); }
.knob-on { transform:translateX(20px); }
.knob-off { transform:translateX(2px); }

/* Log */
.log-panel { border-top:1px solid #2e2e33; background:rgba(0,0,0,.2); }
.log-header { width:100%; display:flex; align-items:center; justify-content:space-between; padding:6px 16px; background:none; border:none; cursor:pointer; color:#71717a; font-size:11px; }
.log-list { padding:0 16px 8px; max-height:112px; overflow-y:auto; }
.log-line { font-family:monospace; font-size:10px; display:flex; gap:8px; padding:1px 0; }
.log-time { color:#71717a; flex-shrink:0; }
.log-level-info { color:#22c55e; flex-shrink:0; width:28px; }
.log-level-warn { color:#eab308; flex-shrink:0; width:28px; }
.log-level-error { color:#ef4444; flex-shrink:0; width:28px; }
.log-level-debug { color:#71717a; flex-shrink:0; width:28px; }
.log-msg { color:#a1a1aa; flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.log-empty { font-size:10px; color:#71717a; padding:6px 0; }

/* Footer */
.footer { display:flex; align-items:center; justify-content:space-between; padding:6px 16px; border-top:1px solid #2e2e33; }
.footer-btn { font-size:11px; padding:4px 10px; border-radius:6px; background:rgba(63,63,70,.2); border:1px solid #2e2e33; color:#a1a1aa; cursor:pointer; transition:all .15s; }
.footer-btn:hover { background:rgba(63,63,70,.4); color:#e4e4e7; }
.footer-btn:disabled { opacity:.3; cursor:not-allowed; }
.version { font-size:10px; color:#71717a; }

/* Modal */
.modal-overlay { position:absolute; inset:0; background:rgba(0,0,0,.6); display:flex; align-items:center; justify-content:center; z-index:50; }
.modal-card { background:#232328; border:1px solid #2e2e33; border-radius:12px; width:100%; max-width:260px; margin:0 16px; }
.modal-header { display:flex; align-items:center; justify-content:space-between; padding:12px 16px; border-bottom:1px solid #2e2e33; }
.modal-title { font-size:14px; font-weight:600; }
.modal-body { padding:16px; display:flex; flex-direction:column; gap:16px; }
.setting-row { display:flex; align-items:center; justify-content:space-between; }
.setting-label { font-size:12px; font-weight:500; }
.setting-desc { font-size:10px; color:#71717a; }
.setting-action { width:100%; padding:8px; font-size:12px; border-radius:8px; background:rgba(63,63,70,.2); border:1px solid #2e2e33; color:#a1a1aa; cursor:pointer; transition:all .15s; }
.setting-action:hover { background:rgba(63,63,70,.4); color:#e4e4e7; }
.setting-quit { width:100%; padding:8px; font-size:12px; border-radius:8px; background:rgba(239,68,68,.15); border:1px solid rgba(239,68,68,.3); color:#ef4444; cursor:pointer; transition:all .15s; }
.setting-quit:hover { background:rgba(239,68,68,.25); }

.spinner { width:16px; height:16px; border:2px solid currentColor; border-top-color:transparent; border-radius:50%; display:inline-block; vertical-align:text-bottom; margin-right:6px; }
</style>
