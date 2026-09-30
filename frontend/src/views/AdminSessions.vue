<template>
  <div>
    <div class="page-head">
      <div>
        <h2 class="page-title">登录设备</h2>
        <p class="page-sub">全部账号的面板登录会话。区分「当前在线」（登录令牌未过期）与「登录过」（历史）；协议用量按账号统计（VLESS/HY2 等），非按设备。</p>
      </div>
    </div>

    <div class="session-toolbar">
      <n-tabs v-model:value="statusFilter" type="line" size="small">
        <n-tab-pane name="all" :tab="`全部 ${sessions.length}`" />
        <n-tab-pane name="online" :tab="`当前在线 ${onlineCount}`" />
        <n-tab-pane name="offline" :tab="`登录过 ${sessions.length - onlineCount}`" />
      </n-tabs>
      <n-input v-model:value="keyword" placeholder="搜索 用户名 / IP / 设备" clearable style="width:220px;" size="small" />
    </div>

    <n-spin :show="loading">
      <n-data-table
        v-if="filtered.length"
        :columns="columns"
        :data="filtered"
        :bordered="false"
        size="small"
        :pagination="{ pageSize: 15 }"
      />
      <n-empty v-else-if="!loading" description="暂无登录会话" style="padding:40px 0;" />
    </n-spin>

    <n-modal v-model:show="usageShow" preset="card" :title="usageTitle" style="width:420px;">
      <n-spin :show="usageLoading">
        <div v-if="usageRows.length" style="display:flex;flex-direction:column;gap:8px;">
          <div v-for="u in usageRows" :key="u.protocol" class="usage-row">
            <span class="usage-proto">{{ protoLabel(u.protocol) }}</span>
            <span class="usage-up">{{ fmtBytes(u.up) }} ↑</span>
            <span class="usage-down">{{ fmtBytes(u.down) }} ↓</span>
          </div>
        </div>
        <n-empty v-else-if="!usageLoading" description="该账号暂无协议用量数据（需先有流量经过）" style="padding:24px 0;" />
      </n-spin>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { NDataTable, NButton, NTag, NTabs, NTabPane, NInput, NSpin, NEmpty, NModal, useMessage, useDialog } from 'naive-ui'
import { apiList, apiGet, apiPost } from '@/api'
import { fmtDateTime, timeAgo, fmtBytes } from '@/utils/format'

const message = useMessage()
const dialog = useDialog()
const sessions = ref<any[]>([])
const loading = ref(false)
const statusFilter = ref<'all' | 'online' | 'offline'>('all')
const keyword = ref('')

const usageShow = ref(false)
const usageLoading = ref(false)
const usageTitle = ref('')
const usageRows = ref<any[]>([])

function parseUA(ua: string): string {
  if (!ua) return '未知设备'
  const os = ua.includes('Windows') ? 'Windows' : ua.includes('Mac') ? 'macOS' : ua.includes('Linux') ? 'Linux' : ua.includes('Android') ? 'Android' : ua.includes('iPhone') || ua.includes('iOS') ? 'iOS' : '未知'
  const br = ua.includes('Edg/') ? 'Edge' : ua.includes('Chrome') ? 'Chrome' : ua.includes('Firefox') ? 'Firefox' : ua.includes('Safari') ? 'Safari' : ua.includes('curl') ? 'curl' : '浏览器'
  return `${os} · ${br}`
}

function protoLabel(p: string): string {
  if (p === 'vless') return 'VLESS'
  if (p === 'hysteria2') return 'Hysteria2 (HY2)'
  if (p === 'vmess') return 'VMess'
  return p || '未知'
}

const onlineCount = computed(() => sessions.value.filter(s => s.online).length)

const filtered = computed(() => {
  const kw = (keyword.value || '').trim().toLowerCase()
  return sessions.value.filter(s => {
    if (statusFilter.value === 'online' && !s.online) return false
    if (statusFilter.value === 'offline' && s.online) return false
    if (kw) {
      const hay = `${s.username || ''} ${s.ip || ''} ${s.user_agent || ''}`.toLowerCase()
      if (!hay.includes(kw)) return false
    }
    return true
  })
})

async function load() {
  loading.value = true
  try { sessions.value = (await apiList('/api/admin/sessions')) || [] }
  catch (e: any) { message.error(e.message) }
  finally { loading.value = false }
}

function revoke(s: any) {
  dialog.warning({
    title: '强制下线设备',
    content: `确定下线「${s.username}」的这个会话（${parseUA(s.user_agent)} · ${s.ip || '无IP'}）？对方将需要重新登录。`,
    positiveText: '下线', negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await apiPost(`/api/admin/sessions/${s.id}/revoke`)
        message.success('已下线')
        await load()
      } catch (e: any) { message.error(e.message) }
    },
  })
}

async function showUsage(s: any) {
  usageShow.value = true
  usageLoading.value = true
  usageRows.value = []
  usageTitle.value = `${s.username} · 协议用量`
  try {
    const all = (await apiGet<any[]>('/api/admin/protocol-usage?user_id=' + s.user_id)) || []
    usageRows.value = all.filter(u => u.user_id === s.user_id)
  } catch (e: any) {
    usageRows.value = []
    message.error(e.message)
  } finally { usageLoading.value = false }
}

const columns = [
  { title: '用户', key: 'username', width: 120, render: (r: any) => h('span', { style: 'font-weight:600;' }, r.username) },
  { title: '设备', key: 'user_agent', minWidth: 150, render: (r: any) => parseUA(r.user_agent) },
  { title: 'IP', key: 'ip', width: 130 },
  { title: '登录时间', key: 'created_at', width: 150, render: (r: any) => fmtDateTime(r.created_at) },
  { title: '最后活跃', key: 'last_seen', width: 100, render: (r: any) => timeAgo(r.last_seen) },
  {
    title: '状态', key: 'online', width: 90,
    render: (r: any) => r.online
      ? h(NTag, { type: 'success', size: 'tiny', bordered: false }, { default: () => '在线' })
      : h(NTag, { type: 'default', size: 'tiny', bordered: false }, { default: () => '登录过' }),
  },
  {
    title: '操作', key: 'act', width: 150,
    render: (r: any) => h('div', { style: 'display:flex;gap:6px;' }, [
      h(NButton, { size: 'tiny', onClick: () => showUsage(r) }, { default: () => '协议用量' }),
      h(NButton, { size: 'tiny', type: 'error', onClick: () => revoke(r) }, { default: () => '下线' }),
    ]),
  },
]

onMounted(load)
</script>

<style scoped>
.session-toolbar { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.usage-row { display: flex; align-items: center; gap: 12px; padding: 8px 10px; background: var(--bg-soft); border: 1px solid var(--border); border-radius: 8px; }
.usage-proto { font-weight: 650; width: 150px; }
.usage-up { color: #18a058; font-size: 12.5px; }
.usage-down { color: var(--text-2); font-size: 12.5px; }
</style>