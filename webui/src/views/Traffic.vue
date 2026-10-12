<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import type { UsageSummary, UsagePoint, UsageRecord } from '@/types'
import { toast } from '@/lib/toast'
import { int, abbr, credits as fmtCredits, dt, pct } from '@/lib/format'
import WPage from '@/components/ui/WPage.vue'
import WCard from '@/components/ui/WCard.vue'
import WTabs from '@/components/ui/WTabs.vue'
import WStat from '@/components/ui/WStat.vue'
import WButton from '@/components/ui/WButton.vue'
import WInput from '@/components/ui/WInput.vue'
import WSelect from '@/components/ui/WSelect.vue'
import WModal from '@/components/ui/WModal.vue'
import WSpinner from '@/components/ui/WSpinner.vue'
import WIcon from '@/components/ui/WIcon.vue'
import TrendChart from '@/components/TrendChart.vue'
import BarList from '@/components/BarList.vue'
import RequestLogTable from '@/components/RequestLogTable.vue'
import RequestLogDetail from '@/components/RequestLogDetail.vue'
import SessionsPanel from '@/components/SessionsPanel.vue'

const route = useRoute()
const router = useRouter()
const tab = ref(['logs', 'sessions'].includes(String(route.query.tab)) ? String(route.query.tab) : 'analytics')
watch(tab, (t) => { router.replace({ query: t !== 'analytics' ? { tab: t } : {} }); loadTab(t) })
watch(() => route.query.tab, (value) => { tab.value = ['logs', 'sessions'].includes(String(value)) ? String(value) : 'analytics' })

// ================= 分析 =================
const summary = ref<UsageSummary | null>(null)
const points = ref<UsagePoint[]>([])
const granularity = ref('hour')
const metric = ref<string>('count')
const anaLoading = ref(true)

async function loadAnalytics() {
  anaLoading.value = true
  try {
    const [s, ts] = await Promise.all([
      api.usageSummary(),
      api.usageTimeseries(granularity.value, granularity.value === 'hour' ? 24 : 30),
    ])
    summary.value = s
    points.value = ts.data || []
  } finally {
    anaLoading.value = false
  }
}
watch(granularity, loadAnalytics)

const protocolBars = computed(() =>
  (summary.value?.by_protocol || []).map((p) => ({ label: p.protocol, value: p.count, sub: abbr(p.tokens) + ' tok' })),
)
const modelBars = computed(() =>
  (summary.value?.by_model || []).slice(0, 10).map((m) => ({ label: m.model, value: m.count, sub: abbr(m.tokens) + ' tok' })),
)
const appBars = computed(() =>
  (summary.value?.by_app || []).map((a) => ({ label: a.app || '(未命名)', value: a.count, sub: abbr(a.tokens) + ' tok' })),
)
const accountBars = computed(() =>
  (summary.value?.by_account || []).map((a) => ({
    label: (a.label || a.account_uid.slice(0, 8)) + (a.removed ? ' (已删除)' : ''),
    value: a.count,
    sub: fmtCredits(a.credits) + ' 额度',
  })),
)

// 缓存命中：仅上游上报过 cached_tokens 的请求参与统计。
// cached / (cached + 未命中输入)；输入里包含缓存的增量部分才是"可命中"的总量。
const cacheStats = computed(() => {
  const c = summary.value?.cache
  if (!c || !c.known_rows || c.hit_rate == null) return null
  return { rows: c.known_rows, hitRate: pct(c.hit_rate), cached: c.cached_tokens, uncached: c.uncached_tokens }
})

// ================= 日志 =================
const records = ref<UsageRecord[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const logLoading = ref(false)
const logError = ref('')
const detailError = ref('')
let logVersion=0, detailVersion=0, disposed=false
onUnmounted(()=>{disposed=true;logVersion++;detailVersion++})
const filters = ref<{ protocol?: string; model?: string; app_name?: string; status?: string; search?: string }>({})
const filterOpts = ref<{ protocols: string[]; models: string[]; apps: string[]; apps_history: string[]; statuses: string[] }>({
  protocols: [], models: [], apps: [], apps_history: [], statuses: [],
})
const searchInput = ref('')

// Channel-scoped labels prevent identically named resource IDs from colliding.
const labelMap = ref<Record<string, string>>({})
const labelProviders = ref<string[]>([])
const defaultLabelProvider = ref('workbuddy')
async function loadAccountLabels() {
  try {
    const providers = (await api.getProviders()).providers
    labelProviders.value = providers.map(p => p.name)
    defaultLabelProvider.value = providers.find(p => p.default)?.name || ''
    const results = await Promise.allSettled(providers.map(p => api.getProvider(p.name)))
    const next: Record<string, string> = {}
    results.forEach((result, i) => {
      const prefix = providers[i].name + '/'
      if (result.status === 'rejected') {
        for (const [key, value] of Object.entries(labelMap.value)) if (key.startsWith(prefix)) next[key] = value
        return
      }
      const resources = result.value.resources || []
      const counts = new Map<string, number>()
      for (const resource of resources) counts.set(resource.label, (counts.get(resource.label) || 0) + 1)
      for (const resource of resources) next[prefix + resource.id] = resource.label + ((counts.get(resource.label) || 0) > 1 ? ' · ' + resource.id.slice(0,8) : '')
    })
    labelMap.value = next
  } catch { /* Keep the last successful label snapshot. */ }
}
function acctLabel(uid?: string | null, model = ''): string {
  if (!uid) return '—'
  const channel = labelProviders.value.find(name => model.startsWith(name + '/')) || defaultLabelProvider.value
  return labelMap.value[channel + '/' + uid] || uid.slice(0, 8)
}

async function loadFilters() {
  const f = await api.usageFilters()
  filterOpts.value = { protocols: f.protocols || [], models: f.models || [], apps: f.apps || [], apps_history: f.apps_history || [], statuses: f.statuses || [] }
}
async function loadLogs() {
  const version=++logVersion
  logLoading.value=true;logError.value=''
  try {
    const res=await api.usageRecent(page.value,pageSize,{...filters.value})
    if(disposed || version!==logVersion)return
    records.value=res.records || [];total.value=res.total || 0
  } catch(e:any) { if(!disposed && version===logVersion)logError.value=e?.message || '查询调用记录失败' }
  finally { if(!disposed && version===logVersion)logLoading.value=false }
}
function applyFilter() {
  filters.value.search = searchInput.value.trim() || undefined
  // page 变化会触发 watch 加载；已在第 1 页时手动加载一次，避免重复请求
  if (page.value === 1) loadLogs()
  else page.value = 1
}
function resetFilter() {
  filters.value = {}
  searchInput.value = ''
  if (page.value === 1) loadLogs()
  else page.value = 1
}
watch(page, loadLogs)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

// 详情
const detailOpen = ref(false)
const detail = ref<UsageRecord | null>(null)
const detailLoading = ref(false)
async function openDetail(id: number) {
  const version=++detailVersion
  detailOpen.value=true;detailLoading.value=true;detail.value=null;detailError.value=''
  try { const result=await api.usageDetail(id);if(!disposed && version===detailVersion)detail.value=result.record }
  catch(e:any){if(!disposed && version===detailVersion)detailError.value=e?.message || '查询调用详情失败'}
  finally {if(!disposed && version===detailVersion)detailLoading.value=false}
}

// 导出 CSV（遍历全部分页）
const exporting = ref(false)
async function exportCSV() {
  exporting.value = true
  try {
    const exportFilters={...filters.value}
    const all: UsageRecord[] = []
    let p = 1
    for (;;) {
      const res = await api.usageRecent(p, 100, exportFilters)
      all.push(...(res.records || []))
      if (all.length >= (res.total || 0) || !res.records?.length) break
      p++
    }
    const head = ['时间','请求标识','调用模型','实际模型','协议','Key','账号','输入Tokens','输出Tokens','缓存读取Tokens','最终命中消耗','消耗渠道','总耗时ms','排队ms','账号等待ms','首响应ms','上游尝试次数','客户端HTTP状态','结果','结束原因']
    const cell=(v: unknown) => {
      let text=String(v ?? '')
      if(typeof v==='string' && /^[=+@\-\t\r]/.test(text))text="'"+text
      return '"'+text.replace(/"/g,'""')+'"'
    }
    const lines = all.map(r=>{
      const p=r.diagnostics?.performance, known=r.tokens_known===true || r.tokens_known===1
      const physical=r.diagnostics?.physical_model || r.model
      const channel=physical.includes('/')?physical.split('/')[0]:'workbuddy'
      return [dt(r.ts,'YYYY-MM-DD HH:mm:ss'),p?.request_id,r.diagnostics?.requested_model || r.model,physical,r.protocol,r.app_name,r.account_uid,
        known?r.input_tokens:null,known?r.output_tokens:null,r.cached_tokens,r.credit_known?r.credits:null,channel,p?.total_ms ?? r.latency_ms,
        p?.queue_ms,p?.account_wait_ms,p?.first_byte_ms,p?.attempts,p?.http_status,r.status,r.diagnostics?.finish_reason].map(cell).join(',')
    })
    const blob = new Blob(['﻿' + [head.join(','), ...lines].join('\n')], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `usage-${dt(Date.now() / 1000, 'YYYYMMDD-HHmm')}.csv`
    a.click()
    URL.revokeObjectURL(url)
    toast.success(`已导出 ${all.length} 条`)
  } finally {
    exporting.value = false
  }
}

function loadTab(value: string) {
  if (value === 'analytics') void loadAnalytics().catch(() => {})
  if (value === 'logs') {
    void loadFilters().catch(() => {})
    void loadAccountLabels()
    void loadLogs().catch(() => {})
  }
}
onMounted(() => loadTab(tab.value))
</script>

<template>
  <WPage title="流量" sub="调用量分析、调用日志与活跃会话账号调整。">
    <template #actions>
      <WTabs v-model="tab" :tabs="[{ key: 'analytics', label: '分析' }, { key: 'logs', label: '日志' }, { key: 'sessions', label: '会话' }]" />
    </template>

    <!-- ============ 分析 ============ -->
    <template v-if="tab === 'analytics'">
      <WSpinner v-if="anaLoading && !summary" center label="加载中" />
      <template v-else>
        <div class="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
          <WStat label="累计请求" :value="int(summary?.total_requests)" tone="brand" />
          <WStat label="累计 Tokens" :value="abbr(summary?.total_tokens)" tone="route" />
          <WStat label="今日请求" :value="int(summary?.today_requests)" tone="live" />
          <WStat label="今日 Tokens" :value="abbr(summary?.today_tokens)" />
        </div>

        <!-- 缓存命中：有可统计样本才展示（qoder 等不上报的上游自动隐藏） -->
        <WCard v-if="cacheStats" title="提示词缓存" class="mb-4">
          <template #actions>
            <span class="text-micro text-faint">基于 {{ int(cacheStats.rows) }} 条已知样本（仅统计缓存数据有效的成功请求）</span>
          </template>
          <div class="flex flex-wrap items-center gap-6">
            <div class="min-w-24">
              <div class="text-micro text-faint">命中率</div>
              <div class="mono text-2xl font-semibold text-live">{{ cacheStats.hitRate }}</div>
            </div>
            <div class="min-w-24">
              <div class="text-micro text-faint">命中 Tokens</div>
              <div class="mono text-small text-ink">{{ int(cacheStats.cached) }}</div>
            </div>
            <div class="min-w-24">
              <div class="text-micro text-faint">未命中 Tokens</div>
              <div class="mono text-small text-muted">{{ int(cacheStats.uncached) }}</div>
            </div>
            <div class="h-2 min-w-40 flex-1 overflow-hidden rounded-full bg-bg">
              <div class="h-full rounded-full bg-live/70" :style="{ width: cacheStats.hitRate || '0%' }" />
            </div>
          </div>
        </WCard>

        <WCard title="调用趋势" class="mb-4">
          <template #actions>
            <WTabs v-model="metric" :tabs="[{ key: 'count', label: '请求数' }, { key: 'tokens', label: 'Tokens' }]" />
            <WTabs v-model="granularity" :tabs="[{ key: 'hour', label: '24 小时' }, { key: 'day', label: '30 天' }]" />
          </template>
          <TrendChart :data="points" :metric="metric" />
        </WCard>

        <div class="grid gap-4 lg:grid-cols-2">
          <WCard title="按协议"><BarList :items="protocolBars" tone="brand" /></WCard>
          <WCard title="按账号"><BarList :items="accountBars" tone="live" /></WCard>
          <WCard title="按模型（Top 10）"><BarList :items="modelBars" tone="route" /></WCard>
          <WCard title="按应用"><BarList :items="appBars" tone="warn" /></WCard>
        </div>
      </template>
    </template>

    <!-- ============ 日志 ============ -->
    <template v-else-if="tab === 'logs'">
      <div class="mb-4 flex flex-wrap items-center gap-2">
        <WInput v-model="searchInput" placeholder="搜索输入/输出/思考内容" class="w-full sm:w-64" @enter="applyFilter">
          <template #prefix><WIcon name="search" :size="16" class="text-faint" /></template>
        </WInput>
        <WSelect v-model="filters.protocol" :options="filterOpts.protocols.map((p) => ({ value: p, label: p }))" placeholder="全部协议" class="w-32" @update:model-value="applyFilter" />
        <WSelect v-model="filters.model" :options="filterOpts.models.map((m) => ({ value: m, label: m }))" placeholder="全部模型" class="w-44" @update:model-value="applyFilter" />
        <WSelect v-model="filters.status" :options="filterOpts.statuses.map((s) => ({ value: s, label: s }))" placeholder="全部状态" class="w-28" @update:model-value="applyFilter" />
        <WButton variant="subtle" size="sm" @click="resetFilter">重置</WButton>
        <div class="ml-auto flex items-center gap-2">
          <WButton variant="ghost" size="sm" :loading="exporting" @click="exportCSV"><WIcon name="download" :size="15" /> 导出</WButton>
          <WButton variant="ghost" size="sm" @click="loadLogs"><WIcon name="refresh" :size="15" /> 刷新</WButton>
        </div>
      </div>

      <p v-if="logError" class="mb-3 text-small text-fault">{{ logError }}</p>
      <WCard flush>
        <WSpinner v-if="logLoading && !records.length" center label="加载中" />
        <RequestLogTable v-else :rows="records" :account-label="acctLabel" admin @detail="openDetail" />
      </WCard>

      <p class="mt-3 text-micro text-faint">按网关请求归并重试；Tokens 为最终命中报告值，各次消耗在详情中展示。历史无请求标识记录逐条保留。</p>
      <!-- 分页 -->
      <div v-if="total > pageSize" class="mt-4 flex items-center justify-between text-small text-muted">
        <span>共 <span class="mono text-ink">{{ int(total) }}</span> 条</span>
        <div class="flex items-center gap-2">
          <WButton size="sm" variant="subtle" :disabled="page <= 1" @click="page--">上一页</WButton>
          <span class="mono">{{ page }} / {{ totalPages }}</span>
          <WButton size="sm" variant="subtle" :disabled="page >= totalPages" @click="page++">下一页</WButton>
        </div>
      </div>
    </template>

    <!-- 记录详情 -->
    <SessionsPanel v-if="tab === 'sessions'" />

    <WModal v-model:open="detailOpen" size="xl" title="调用详情">
      <WSpinner v-if="detailLoading" label="加载中" />
      <p v-else-if="detailError" class="text-small text-fault">{{ detailError }}</p>
      <RequestLogDetail v-else-if="detail" :record="detail" :account-label="acctLabel" admin />
      <template #footer><WButton variant="primary" @click="detailOpen = false">关闭</WButton></template>
    </WModal>
  </WPage>
</template>
