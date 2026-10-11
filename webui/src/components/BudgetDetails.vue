<script setup lang="ts">
import { useId } from 'vue'
import type { Overview } from '@/types'
import { abbr, credits } from '@/lib/format'

defineProps<{ prediction?: Overview['prediction'] }>()
const popoverID = useId()
</script>

<template>
  <button type="button" :popovertarget="popoverID" class="whitespace-nowrap text-small text-brand hover:underline">续航明细</button>
  <Teleport to="body">
    <div :id="popoverID" popover="auto" role="dialog" aria-label="各渠道预计续航" class="budget-popover glass rounded-xl border border-line p-5 text-ink shadow-glass">
      <div class="mb-3 flex items-center justify-between gap-4">
        <h3 class="font-semibold">各渠道预计续航</h3>
        <button type="button" :popovertarget="popoverID" popovertargetaction="hide" aria-label="关闭续航明细" class="text-small text-muted hover:text-ink">关闭</button>
      </div>
      <p class="mb-4 text-small text-muted">总数为已覆盖付费余额的 Tokens 折算，各池按近期付费模型使用比例独立估算。不同池的余额不能互换，免费调用不计为无限续航，也不放大付费预测。</p>
      <p v-if="prediction?.error" class="mb-3 text-small text-warn">{{ prediction.error }}</p>
      <div v-for="channel in prediction?.providers || []" :key="channel.provider" class="border-t border-line py-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="font-medium">{{ channel.label }}</span>
          <span class="mono text-brand">{{ channel.predicted_tokens == null ? '暂不可估算' : abbr(channel.predicted_tokens, 2) + ' Tokens' }}</span>
        </div>
        <p v-if="channel.reason" class="mt-1 text-small text-faint">{{ channel.reason }}</p>
        <p v-if="channel.free_models.length" class="mt-2 break-words text-small text-live">目录明确免费：{{ channel.free_models.join('、') }}（实际可用性仍受上游资格、冷却与限流约束）</p>
        <div v-for="(pool, index) in channel.pools" :key="index" class="mt-3 rounded-lg bg-bg/50 p-3 text-small">
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
      </div>
      <p class="mt-3 text-micro text-faint">至少 5 次、1,000 Tokens 的有效付费样本才输出预测；优先近 7 天，样本不足参考近 30 天或同计费池。模型切换、缓存比例、价格与额度到期都会影响实际续航。</p>
    </div>
  </Teleport>
</template>

<style scoped>
.budget-popover { background-color: rgb(var(--c-surface)); width: min(640px, calc(100vw - 32px)); max-height: calc(100dvh - 48px); overflow-y: auto; }
.budget-popover::backdrop { background: rgb(0 0 0 / 20%); }
</style>
