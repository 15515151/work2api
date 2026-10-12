<script setup lang="ts">
import type { UsageRecord } from '../types'
import WTable, { type Column } from './ui/WTable.vue'
import WButton from './ui/WButton.vue'
import WEmpty from './ui/WEmpty.vue'
import { dt, int, latency } from '../lib/format'
defineProps<{ rows: UsageRecord[]; accountLabel?: (uid: string, model: string) => string; admin?: boolean }>()
defineEmits<{ detail: [id: number] }>()
const columns: Column[] = [
 {key:'ts',label:'时间',nowrap:true},{key:'model',label:'调用 → 实际模型'},
 {key:'app_name',label:'Key / 协议',nowrap:true},{key:'tokens',label:'Tokens（入 / 出）',align:'right',nowrap:true},
 {key:'timing',label:'耗时 / 等待',align:'right',nowrap:true},{key:'status',label:'结果',nowrap:true},{key:'actions',label:'',align:'right',nowrap:true},
]
function known(r: {tokens_known?: unknown}) { return r.tokens_known === true || r.tokens_known === 1 }
function status(r: {status?: string}) { return r.status==='ok'?'成功':r.status==='error'?'失败':r.status==='incomplete'?'输出未完成':r.status }
</script>
<template>
 <WTable :columns="columns" :rows="rows" row-key="id" min-width="850px">
  <template #cell-ts="{ row }"><span class="mono text-micro">{{ dt(row.ts,'MM-DD HH:mm:ss') }}</span></template>
  <template #cell-model="{ row }">
   <div class="max-w-72 truncate text-small" :title="row.diagnostics?.requested_model || row.model">{{ row.diagnostics?.requested_model || row.model }}</div>
   <div v-if="row.diagnostics?.physical_model && row.diagnostics.physical_model !== (row.diagnostics.requested_model || row.model)" class="mono max-w-72 truncate text-micro text-faint" :title="row.diagnostics.physical_model">→ {{ row.diagnostics.physical_model }}</div>
   <div v-if="admin && row.account_uid" class="max-w-72 truncate text-micro text-faint">{{ accountLabel?.(row.account_uid, row.model) || row.account_uid.slice(0,8) }}</div>
  </template>
  <template #cell-app_name="{ row }"><div class="max-w-40 truncate" :title="row.app_name">{{ row.app_name || '未命名 Key' }}</div><div class="text-micro text-faint">{{ row.protocol }}<template v-if="row.diagnostics?.performance?.response_stream != null"> · {{ row.diagnostics.performance.response_stream ? '流式' : '非流式' }}</template></div></template>
  <template #cell-tokens="{ row }"><div class="mono">{{ known(row) ? int(row.input_tokens) : '—' }} / {{ known(row) ? int(row.output_tokens) : '—' }}</div><div class="text-micro text-faint">缓存读 {{ row.cached_tokens == null?'—':int(row.cached_tokens) }}</div></template>
  <template #cell-timing="{ row }"><div class="mono">{{ latency(row.diagnostics?.performance?.total_ms ?? row.latency_ms) }}</div><div v-if="row.diagnostics?.performance" class="text-micro text-faint">排队 {{ latency(row.diagnostics.performance.queue_ms) }} · 账号 {{ latency(row.diagnostics.performance.account_wait_ms) }}</div></template>
  <template #cell-status="{ row }"><div :class="row.status==='ok'?'text-live':row.status==='error'?'text-fault':'text-warn'">{{ status(row) }}<span v-if="row.diagnostics?.performance?.http_status" class="mono ml-1 text-micro">{{ row.diagnostics.performance.http_status }}</span></div><div v-if="(row.diagnostics?.performance?.attempts || row.logged_attempts || 0)>1" class="text-micro text-faint">{{ row.diagnostics?.performance?.attempts || row.logged_attempts }} 次上游尝试</div></template>
  <template #cell-actions="{ row }"><WButton size="sm" variant="subtle" @click="$emit('detail',row.id)">详情</WButton></template>
  <template #empty><WEmpty title="没有调用记录" hint="调整筛选条件，或等待新的请求。" /></template>
 </WTable>
</template>
