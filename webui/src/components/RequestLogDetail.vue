<script setup lang="ts">
import { computed } from 'vue'
import type { UsageRecord } from '../types'
import { dt, int, latency } from '../lib/format'
const props=defineProps<{ record: UsageRecord; admin?: boolean; accountLabel?: (uid: string, model: string) => string }>()
const d=computed(()=>props.record.diagnostics), p=computed(()=>d.value?.performance)
const known=computed(()=>props.record.tokens_known===true || props.record.tokens_known===1)
const budget=(v?:Record<string,number>)=>v && Object.keys(v).length?Object.entries(v).map(([k,n])=>k + ': ' + n).join(' · '):'未显式指定'
const compatibility=computed(()=>({original:'原文',identity:'固定身份兼容',client_metadata:'客户端身份与归因精简'} as Record<string,string>)[d.value?.compatibility || ''] || '未记录')
const channel=(model:string)=>model.includes('/')?model.split('/')[0]:'workbuddy'
const unit=(model:string)=>channel(model)==='opencode'?'上游报告消耗（单位未确认）':channel(model) + ' 渠道积分'
function stageLabel(stage: {headers_finished:boolean;http_status:number;header_wait_ms:number}) { return stage.headers_finished ? 'HTTP '+(stage.http_status || '无响应')+' · 响应头等待 '+stage.header_wait_ms+' ms' : '未收到响应头' }
</script>
<template>
 <div class="space-y-5 text-small">
  <section class="rounded-xl border border-line p-4">
   <h4 class="mb-3 font-semibold">请求与路由</h4>
   <dl class="grid gap-3 sm:grid-cols-2">
    <div><dt class="text-micro text-faint">调用模型</dt><dd class="mono break-all">{{ d?.requested_model || record.model }}</dd></div>
    <div><dt class="text-micro text-faint">实际渠道模型</dt><dd class="mono break-all">{{ d?.physical_model || record.model }}</dd></div>
    <div><dt class="text-micro text-faint">Key / 协议</dt><dd class="break-all">{{ record.app_name || '未命名 Key' }} · {{ record.protocol }}<template v-if="p?.response_stream != null"> · {{ p.response_stream?'流式':'非流式' }}</template></dd></div>
    <div><dt class="text-micro text-faint">时间 / 结果</dt><dd>{{ dt(record.ts,'YYYY-MM-DD HH:mm:ss') }} · {{ record.status }}<template v-if="p?.http_status"> · HTTP {{ p.http_status }}</template></dd></div>
    <div v-if="admin && record.account_uid"><dt class="text-micro text-faint">最终账号 / 用户</dt><dd class="break-all">{{ accountLabel?.(record.account_uid,record.model) || record.account_uid }} · {{ record.user_id ? '用户 #' + record.user_id : '管理员 Key' }}</dd></div>
    <div><dt class="text-micro text-faint">结束原因 / 思考强度</dt><dd>{{ d?.finish_reason || '未上报' }} · {{ record.reasoning_effort || '未记录' }}</dd></div>
   </dl>
   <div v-if="p?.request_id" class="mt-3 rounded-lg bg-elevated p-3"><span class="text-micro text-faint">网关请求标识（反馈问题时提供）</span><div class="mono select-all break-all">{{ p.request_id }}</div></div>
  </section>
  <section class="rounded-xl border border-line p-4"><h4 class="mb-3 font-semibold">用量与性能</h4>
   <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
    <div><div class="text-micro text-faint">输入 / 输出 Tokens</div><div class="mono">{{ known?int(record.input_tokens):'—' }} / {{ known?int(record.output_tokens):'—' }}</div></div>
    <div><div class="text-micro text-faint">缓存读取 Tokens</div><div class="mono">{{ record.cached_tokens == null?'—':int(record.cached_tokens) }}</div><p v-if="record.cached_tokens != null && (record.cached_tokens<0 || record.cached_tokens>record.input_tokens)" class="text-micro text-warn">缓存数值超出输入范围</p></div>
    <div v-if="admin"><div class="text-micro text-faint">最终命中实际消耗</div><div class="mono">{{ record.credit_known && record.credits!=null?record.credits:'—' }}</div><p class="text-micro text-faint">{{ unit(d?.physical_model || record.model) }}</p></div>
    <div><div class="text-micro text-faint">整次请求耗时</div><div class="mono">{{ latency(p?.total_ms ?? record.latency_ms) }}</div></div>
    <div><div class="text-micro text-faint">排队 / 账号等待</div><div class="mono">{{ p?latency(p.queue_ms)+' / '+latency(p.account_wait_ms):'—' }}</div></div>
    <div><div class="text-micro text-faint">首响应 / 上游执行</div><div class="mono">{{ p?.first_byte_ms==null?'—':latency(p.first_byte_ms) }} / {{ p?.execution_ms==null?'—':latency(p.execution_ms) }}</div></div>
   </div>
   <p class="mt-3 text-micro text-faint">首响应是网关首字节，包含协议事件，不能作为模型首个 token；“—”表示未记录或上游未上报。重试记录共享整次请求耗时，不累加。</p>
  </section>
  <section v-if="d" class="rounded-xl border border-line p-4 space-y-2"><h4 class="font-semibold">协议处理</h4><div>客户端输出预算：{{ budget(d.requested_output_limits) }}</div><div>实际发送预算：{{ budget(d.effective_output_limits) }}</div><div>输入兼容：{{ compatibility }}</div><div v-if="d.error_kind">故障阶段 / 分类：{{ d.upstream_started?'上游调用阶段':'调用前校验' }} · {{ d.error_kind }}</div></section>
  <section v-if="p?.attempt_stages?.length" class="rounded-xl border border-line p-4"><h4 class="mb-3 font-semibold">上游尝试阶段 · {{ p.attempts }} 次</h4><div class="space-y-2"><div v-for="stage in p.attempt_stages" :key="stage.number" class="rounded-lg bg-elevated p-3">#{{ stage.number }} · {{ stageLabel(stage) }}<div v-if="stage.upstream_request_id" class="mono mt-1 select-all break-all text-micro text-faint">上游请求标识：{{ stage.upstream_request_id }}</div></div></div><p v-if="p.attempts>p.attempt_stages.length" class="mt-2 text-micro text-faint">阶段信息最多保留前 16 次。</p></section>
  <section v-if="record.attempt_records && record.attempt_records.length>1" class="rounded-xl border border-line p-4"><h4 class="mb-3 font-semibold">各次命中记录</h4><div v-for="item in record.attempt_records" :key="item.id" class="mb-2 rounded-lg bg-elevated p-3"><div class="break-all">{{ item.diagnostics?.physical_model || item.model }} · {{ item.status }}<template v-if="admin && item.account_uid"> · {{ accountLabel?.(item.account_uid,item.model) || item.account_uid }}</template><template v-if="admin && item.credit_known && item.credits!=null"> · {{ item.credits }} {{ unit(item.model) }}</template></div><div v-if="item.error" class="mt-1 whitespace-pre-wrap break-all text-micro text-fault">{{ item.error }}</div></div><p class="text-micro text-faint">只展开有用量日志的尝试；未生成用量行的重试见上方阶段记录。跨渠道消耗单位不同，不直接相加。</p></section>
  <div v-if="record.error" class="rounded-xl border border-fault/30 bg-fault/5 p-4 whitespace-pre-wrap break-all text-fault">{{ record.error }}</div>
  <template v-if="admin"><details v-for="section in [{title:'输入（已保存片段）',text:record.input_content},{title:'思考内容（已保存片段）',text:record.reasoning_content},{title:'输出（已保存片段）',text:record.output_content}].filter(v=>v.text)" :key="section.title" class="rounded-xl border border-line p-4"><summary class="cursor-pointer font-semibold">{{ section.title }}</summary><pre class="mono mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all text-micro">{{ section.text }}</pre></details></template>
 </div>
</template>
