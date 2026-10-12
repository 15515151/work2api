<script setup lang="ts">
import { ref } from 'vue'
import WModal from './ui/WModal.vue'
import type { Overview } from '@/types'
import { abbr, credits } from '@/lib/format'

defineProps<{ prediction?: Overview['prediction'] }>()
const open = ref(false)
</script>

<template>
  <button type="button" class="whitespace-nowrap text-small text-brand hover:underline" @click="open = true">续航明细</button>
  <WModal v-model:open="open" title="各渠道预计续航" size="xl">
      <p class="mb-4 text-small text-muted">总数为已覆盖付费余额的 Tokens 折算，各池按近期付费模型使用比例独立估算。不同池的余额不能互换，免费调用不计为无限续航，也不放大付费预测。</p>
      <p v-if="prediction?.error" class="mb-3 text-small text-warn">{{ prediction.error }}</p>
      <div class="grid items-start gap-4 lg:grid-cols-2">
      <article v-for="channel in prediction?.providers || []" :key="channel.provider" class="min-w-0 rounded-xl border border-line bg-surface p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="font-medium">{{ channel.label }}</span>
          <span class="mono text-brand">{{ channel.predicted_tokens == null ? '暂不可估算' : abbr(channel.predicted_tokens, 2) + ' Tokens' }}</span>
        </div>
        <p v-if="channel.reason" class="mt-1 text-small text-faint">{{ channel.reason }}</p>
        <p v-if="channel.free_models.length" class="mt-2 break-words text-small text-live">目录明确免费：{{ channel.free_models.join('、') }}（实际可用性仍受上游资格、冷却与限流约束）</p>
        <div v-for="(pool, index) in channel.pools" :key="index" class="mt-3 min-w-0 rounded-lg border border-line bg-elevated p-3 text-small">
          <div class="flex flex-wrap justify-between gap-2">
            <span class="break-words text-muted">{{ pool.label }}</span>
            <span class="mono whitespace-nowrap">{{ pool.predicted_tokens == null ? '—' : abbr(pool.predicted_tokens, 2) + ' Tokens' }}</span>
          </div>
          <p class="mt-1 text-muted">可估算余额 {{ pool.remaining == null ? '未知' : credits(pool.remaining) + ' ' + pool.unit }}<span v-if="pool.excluded_credits > 0"> · 已剔除过期或适用范围未确认额度 {{ credits(pool.excluded_credits) }}</span></p>
          <p v-if="pool.status === 'estimated'" class="mt-1 text-faint">近 {{ pool.window_days }} 天 · {{ pool.samples }} 次付费样本 · {{ pool.scope === 'account' ? '本账号' : '同计费池' }} · 每{{ pool.unit }}约 {{ abbr(pool.tokens_per_unit || 0) }} Tokens</p>
          <p v-else class="mt-1 text-faint">{{ pool.reason }}</p>
          <p v-if="pool.status === 'estimated' && pool.legacy_samples > 0" class="mt-1 text-warn">含 {{ pool.legacy_samples }} 次旧版历史记录：消费及 Tokens 数值完整，但缺少上游用量确认标记，仅作参考估算。</p>
          <p v-if="pool.stale || pool.deferred" class="mt-1 text-warn">{{ [pool.stale ? '余额待刷新，预测可能滞后' : '', pool.deferred ? '账号暂时冷却，恢复前不可调用' : ''].filter(Boolean).join('；') }}</p>
          <p v-if="pool.free_tokens_observed > 0" class="mt-1 text-faint">近 7 天明确免费调用 {{ abbr(pool.free_tokens_observed) }} Tokens</p>
          <p v-if="pool.zero_cost_unconfirmed_tokens > 0 || pool.unknown_cost_calls > 0" class="mt-1 text-faint">存在消费为零但未确认免费、或消费未知的调用，未用于付费成本校准。</p>
        </div>
      </article>
      </div>
      <p v-if="!prediction?.providers?.length" class="py-6 text-center text-small text-faint">暂无续航明细，请等待概览数据刷新。</p>
      <p class="mt-3 text-micro text-faint">至少 5 次、1,000 Tokens 的有效付费样本才输出预测；优先近 7 天，样本不足参考近 30 天或同计费池。模型切换、缓存比例、价格与额度到期都会影响实际续航。</p>
  </WModal>
</template>
