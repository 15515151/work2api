<script setup lang="ts">
// 管理台与门户共用的模型参数展示；操作由包装层插槽提供，本组件不请求管理接口。
import { computed } from 'vue'
import WTag from '@/components/ui/WTag.vue'
import { int } from '@/lib/format'
import { providerMeta } from '@/lib/providers'

const props = defineProps<{ model: Record<string, any>; provider: string; reserveDesc?: boolean; reserveEfforts?: boolean; reserveCost?: boolean }>()
const m = computed(() => props.model)
const pm = computed(() => providerMeta(props.provider))
const multimodal = computed(() => m.value.modality === 'multimodal' || m.value.vision || m.value.supports_image)
const reasoning = computed(() => {
  const value = m.value.reasoning
  if (value && typeof value === 'object') return value
  return { supportsReasoning: value === true || m.value.is_reasoning === true }
})
const ctx = computed(() => m.value.context_length ?? m.value.context)
const maxOut = computed(() => m.value.max_output_tokens ?? m.value.max_output)
const bench = computed(() => {
  const data = m.value.benchmark
  return data && [data.intelligence_index, data.coding_index, data.math_index].some(knownScore) ? data : null
})
function knownScore(v: unknown): boolean {
  return (typeof v === 'number' || typeof v === 'string' && v.trim() !== '') && Number.isFinite(Number(v))
}
const fmtScore = (v: unknown) => knownScore(v) ? Number(v).toFixed(1) : '—'

// 每站点成本系数（credits_by_region）。同模型两站点不同价时分列展示并标出更便宜的
// 那个（成本优先选号会用它）。全相同或只有一个站点则回退单值 m.credits。
const finiteCost = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v >= 0
const siteName = (s: string) => (s === 'domestic' ? '国内' : s === 'international' ? '国际' : s)
const costEntries = computed<{ label: string; v: number; cheapest: boolean }[] | null>(() => {
  const cbr = m.value.credits_by_region as Record<string, number> | undefined
  if (!cbr) return null
  const es = Object.entries(cbr).filter(([, v]) => finiteCost(v))
  if (es.length < 2 || new Set(es.map(([, v]) => v)).size < 2) return null
  const min = Math.min(...es.map(([, v]) => v))
  return es
    .sort((a, b) => a[1] - b[1])
    .map(([s, v]) => ({ label: siteName(s), v, cheapest: v === min }))
})
// 指标格里显示的成本：多站点时取最低（即成本优先实际会用到的价），否则用合并值。
const displayCredits = computed(() => {
  const cbr = m.value.credits_by_region as Record<string, number> | undefined
  const prices = cbr ? Object.values(cbr).filter(finiteCost) : []
  if (prices.length) return Math.min(...prices)
  return m.value.credits
})

const costMetric = computed(() => {
  if (props.provider === 'qoder') return {
    value: finiteCost(m.value.price_factor) ? `×${m.value.price_factor}` : '-',
    label: '成本系数',
    title: '上游模型目录的相对成本系数，仅在该渠道及相应计费规则内比较，跨渠道数字不能直接比较；不是货币单价，缺失时不推断为免费。',
  }
  if (props.provider === 'opencode') return {
    value: finiteCost(m.value.input_cost) && finiteCost(m.value.output_cost) ? `$${m.value.input_cost} / $${m.value.output_cost}` : '-',
    label: '输入 / 输出价格',
    title: 'models.dev 的 Zen 参考价，单位为美元/百万 tokens；Go 订阅与实际账单以渠道为准。',
  }
  if (props.provider === 'trae') return {
    value: '-', label: '成本系数',
    title: '尚未获取可确认的模型成本系数，目录权益消耗值不能作为计费倍率。',
  }
  return { value: finiteCost(displayCredits.value) ? `×${displayCredits.value}` : '-', label: '成本系数', title: '上游目录成本系数；仅在该渠道及相应计费规则内比较，跨渠道数字不能直接比较；活动或时段优惠以实际扣费为准。' }
})
const nativeProtocol = computed(() => ({
  chat: 'Chat', messages: 'Messages', responses: 'Responses',
  openai: 'Chat', anthropic: 'Messages', systemone: 'SystemOne',
} as Record<string, string>)[String(m.value.native_protocol || '').toLowerCase()])

// 思考档位（对齐上游 Models.vue reasoningEfforts）：优先用 supportedEfforts，
// 否则退回单个 defaultEffort；可关思考则补一个 off。
const efforts = computed<string[]>(() => {
  const r = reasoning.value
  let list: string[] = Array.isArray(r.supportedEfforts) ? [...r.supportedEfforts] : []
  if (!list.length && r.defaultEffort) list = [r.defaultEffort]
  if (r.canDisableThinking && !list.includes('off')) list.push('off')
  return list
})
// 单一思考标签：可关思考 > 仅思考 > 思考。
const reasoningTag = computed<{ text: string; tone: 'brand' | 'live' | 'warn' } | null>(() => {
  const r = reasoning.value
  if (!r.supportsReasoning) return null
  if (r.canDisableThinking) return { text: '可关思考', tone: 'live' }
  if (r.onlyReasoning) return { text: '仅思考', tone: 'warn' }
  return { text: '思考', tone: 'brand' }
})

// SystemOne 决策模型（opencode）：native_protocol=systemone。这类模型的请求体是
// 结构化决策 payload，和聊天消息完全不同，用 /v1/chat|messages|responses 调用会被
// 上游 400。保留展示但明确标注，避免误用（后续如接决策端点可直接用）。
const systemOne = computed(() => String(m.value.native_protocol || '').toLowerCase() === 'systemone')

// ---- 价格时段提醒 ----
// 上游 catalog 的 tags 偶带定价说明（"badge:夜间折扣:#3B82F6"、"限时免费" 等）。
// 这里解析成卡片上的提醒标签：badge:文本:#色号 或纯文本都接受，纯文本用默认色。
// 这类模型按时段高低价计费，选号/成本预估以成本系数列为准，时段内实扣可能更低。
const pricingNotes = computed(() => {
  const tags = (Array.isArray(m.value.tags) ? m.value.tags : []) as string[]
  const notes: { text: string; color: string }[] = []
  for (const t of tags) {
    if (typeof t !== 'string') continue
    if (t.trim() === 'craft') continue // 上游内部标记（手工艺/质量档），与定价无关
    const bd = t.match(/^badge:([^:]+)(?::#([0-9a-fA-F]{3,8}))?$/)
    if (bd) notes.push({ text: bd[1].trim(), color: bd[2] ? `#${bd[2]}` : '' })
    else notes.push({ text: t.trim(), color: '' })
  }
  return notes
})

</script>

<template>
  <!-- h-full + flex-col：网格行内三张卡等高（grid 默认 stretch），各区段用 min-h
       预留高度对齐，footer 用 mt-auto 压到底，跨卡横向成带、可读性更好。 -->
  <div class="glass flex h-full min-w-0 flex-col gap-3 rounded-xl p-4 transition-colors hover:border-brand/40">
    <div class="flex min-h-[2.5rem] items-start justify-between gap-2">
      <div class="min-w-0">
        <div class="truncate font-semibold text-ink" :title="m.name || m.id">{{ m.name || m.id }}</div>
        <div class="mono truncate text-micro text-faint" :title="m.id">{{ m.id }}</div>
      </div>
      <WTag :tone="pm.tone" class="shrink-0 whitespace-nowrap">{{ pm.label }}</WTag>
    </div>

    <div class="flex min-h-[1.625rem] flex-wrap gap-1.5">
      <WTag :tone="multimodal ? 'route' : 'muted'">{{ multimodal ? '多模态' : m.vision === false && m.vision_verified !== false ? '文本' : '图片能力待确认' }}</WTag>
      <WTag v-if="reasoningTag" :tone="reasoningTag.tone">{{ reasoningTag.text }}</WTag>
      <WTag v-if="m.supportsToolCall === true || m.tool_call === true" tone="live">工具调用</WTag>
      <WTag v-if="m.tool_response_multimodal === true" tone="route" title="上游目录声明支持包含图片的工具结果">工具结果图片</WTag>
      <WTag v-if="m.structured_output === true" tone="live" title="上游目录声明结构化输出能力；具体请求参数仍按原生协议处理">结构化输出</WTag>
      <WTag v-if="nativeProtocol && !systemOne" tone="muted" title="网关使用的上游原生协议，不限制网关对外的协议转换">原生 {{ nativeProtocol }}</WTag>
      <WTag v-if="m.catalog_source === 'fallback'" tone="warn" title="尚未获取当前账号的可用模型目录，仅列出保守候选；实际权限以上游为准">目录待验证</WTag>
      <WTag v-else-if="m.catalog_stale" tone="warn" title="目录刷新失败，暂时保留当前账号上次成功获取的结果">目录缓存过期</WTag>
      <WTag v-if="m.pricing_stale" tone="warn" title="参考价格缓存已过期，实际计费以上游账单为准">价格缓存过期</WTag>
      <WTag v-if="systemOne" tone="warn" title="仅 SystemOne 决策协议，不能用普通聊天/消息/Responses 接口调用">SystemOne</WTag>
    </div>

    <!-- 价格时段提醒：上游 catalog 标记的定价说明（夜间折扣/限时免费等）。
         只做提醒不限制调用：这类模型按时段计费，时段内实扣可能低于成本系数。 -->
    <div v-if="pricingNotes.length" class="rounded-lg border border-warn/30 bg-warn/5 px-2.5 py-1.5 text-micro leading-relaxed text-warn">
      <span v-for="(n, i) in pricingNotes" :key="i"
        class="mr-1.5 inline-flex items-center gap-1 rounded-md px-1.5 py-0.5"
        :style="n.color ? { color: n.color, borderColor: n.color + '55', border: '1px solid' } : {}">
        {{ n.text }}
      </span>
      <span class="text-faint">· 按时段计费，实际成本可能低于成本系数</span>
    </div>

    <!-- SystemOne 模型用法提示：这类模型只接受结构化决策 payload，普通聊天/消息/
         Responses 调用会被上游 400，明确标注避免误用。 -->
    <div v-if="systemOne" class="rounded-lg border border-warn/30 bg-warn/5 px-2.5 py-1.5 text-micro leading-relaxed text-warn">
      仅 <span class="mono">SystemOne</span> 决策协议模型：无法用聊天 / 消息 / Responses 接口调用（会返回 400），需按 SystemOne 结构化 payload 请求。
    </div>

    <!-- 思考强度档位：低/中/高/… + 可关思考时的 off。仅当本页有带档位的模型时，
         无档位卡才补等高占位，保持跨卡对齐；全都没有则不占位。 -->
    <div v-if="efforts.length" class="flex min-h-[1.625rem] flex-wrap items-center gap-1">
      <span class="text-micro text-faint">思考强度</span>
      <span
        v-for="e in efforts"
        :key="e"
        class="mono rounded-md px-1.5 py-0.5 text-micro"
        :class="e === reasoning.defaultEffort ? 'bg-brand/15 text-brand ring-1 ring-brand/30' : 'bg-elevated text-muted'"
        :title="e === reasoning.defaultEffort ? '默认档位' : ''"
      >{{ e }}</span>
    </div>
    <div v-else-if="reserveEfforts" class="min-h-[1.625rem]" aria-hidden="true"></div>

    <div class="grid grid-cols-3 gap-2 border-t border-line pt-3 text-center">
      <div>
        <div class="mono text-small font-semibold text-ink">{{ ctx ? int(ctx) : '—' }}</div>
        <div class="text-micro text-faint">上下文</div>
      </div>
      <div>
        <div class="mono text-small font-semibold text-ink">{{ maxOut ? int(maxOut) : '—' }}</div>
        <div class="text-micro text-faint">最大输出</div>
      </div>
      <div>
        <div class="mono break-words text-small font-semibold text-ink" :title="costMetric.title">{{ costMetric.value }}</div>
        <div class="text-micro text-faint" :title="costMetric.title">{{ costMetric.label }}</div>
      </div>
    </div>

    <div v-if="provider === 'opencode' && (finiteCost(m.input_cost) || finiteCost(m.output_cost))" class="text-micro leading-relaxed text-faint">Zen 参考价 · USD / 百万 tokens<span v-if="m.tier === 'go'">；Go 订阅额度另计</span></div>
    <div v-if="provider === 'opencode' && (finiteCost(m.cache_read_cost) || finiteCost(m.cache_write_cost))" class="flex flex-wrap gap-x-3 gap-y-1 text-micro text-faint">
      <span v-if="finiteCost(m.cache_read_cost)">缓存读取 ${{ m.cache_read_cost }}</span>
      <span v-if="finiteCost(m.cache_write_cost)">缓存写入 ${{ m.cache_write_cost }}</span>
      <span>USD / 百万 tokens</span>
    </div>
    <div v-if="m.max_input_tokens > 0" class="text-micro text-faint">最大输入 <span class="mono">{{ int(m.max_input_tokens) }}</span> tokens</div>

    <!-- 多站点成本分列：标出更便宜的站点（成本优先会用它）。同价/单站点则不显示。 -->
    <div v-if="costEntries" class="flex min-h-[1.25rem] flex-wrap items-center gap-x-3 gap-y-0.5 text-micro">
      <span class="text-faint">分站点</span>
      <span v-for="c in costEntries" :key="c.label" class="inline-flex items-center gap-1">
        <span class="text-faint">{{ c.label }}</span>
        <span class="mono" :class="c.cheapest ? 'text-live' : 'text-muted'">x{{ c.v }}</span>
      </span>
    </div>
    <div v-else-if="reserveCost" class="min-h-[1.25rem]" aria-hidden="true"></div>

    <!-- 描述：有描述才占两行；仅当本页存在带描述的模型时，无描述卡才补等高占位，
         使评测框跨卡对齐——全都没描述时不留任何空白。 -->
    <p v-if="m.description" class="line-clamp-2 min-h-[2.25rem] text-micro leading-relaxed text-muted">{{ m.description }}</p>
    <div v-else-if="reserveDesc" class="min-h-[2.25rem]" aria-hidden="true"></div>

    <!-- AA 第三方评测（配置了 key 且匹配到时展示） -->
    <div v-if="bench" class="rounded-lg border border-line bg-elevated/40 p-2.5">
      <div class="mb-1.5 flex items-center justify-between">
        <span class="truncate text-micro text-faint">AA 评测<span v-if="bench.name" class="ml-1 text-faint/70">· {{ bench.name }}</span></span>
        <a :href="bench.aa_url || 'https://artificialanalysis.ai/models'" target="_blank" rel="noopener" class="shrink-0 text-micro text-brand hover:underline">榜单 ↗</a>
      </div>
      <div class="grid grid-cols-3 gap-2 text-center">
        <div>
          <div class="mono text-small font-semibold text-ink">{{ fmtScore(bench.intelligence_index) }}</div>
          <div class="text-micro text-faint">智能</div>
        </div>
        <div>
          <div class="mono text-small font-semibold text-ink">{{ fmtScore(bench.coding_index) }}</div>
          <div class="text-micro text-faint">编码</div>
        </div>
        <div>
          <div class="mono text-small font-semibold text-ink">{{ fmtScore(bench.math_index) }}</div>
          <div class="text-micro text-faint">数学</div>
        </div>
      </div>
    </div>

    <slot />
  </div>
</template>
