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
      <p class="mb-4 text-small text-muted">按余额与近期付费模型使用情况估算，免费调用不计入续航总数。</p>
      <p v-if="prediction?.error" class="mb-3 text-small text-warn">{{ prediction.error }}</p>
      <div class="grid items-start gap-4 lg:grid-cols-2">
      <article v-for="channel in prediction?.providers || []" :key="channel.provider" class="min-w-0 rounded-xl border border-line bg-surface p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="font-medium">{{ channel.label }}</span>
          <span class="mono text-brand">{{ channel.predicted_tokens == null ? '暂不可估算' : abbr(channel.predicted_tokens, 2) + ' Tokens' }}</span>
        </div>
        <p v-if="channel.reason" class="mt-1 text-small text-faint">{{ channel.reason }}</p>
        <details v-if="channel.free_models.length" class="mt-2 text-small"><summary class="cursor-pointer text-live">免费模型 · {{ channel.free_models.length }}</summary><p class="mt-1 break-words text-muted">{{ channel.free_models.join('、') }}</p></details>
        <div v-for="(pool, index) in channel.pools" :key="index" class="mt-3 min-w-0 rounded-lg border border-line bg-elevated p-3 text-small">
          <div class="flex flex-wrap justify-between gap-2">
            <span class="break-words text-muted">{{ pool.label }}</span>
            <span class="mono whitespace-nowrap">{{ pool.predicted_tokens == null ? '—' : abbr(pool.predicted_tokens, 2) + ' Tokens' }}</span>
          </div>
          <p class="mt-1 text-muted">参与估算余额 {{ pool.remaining == null ? '未知' : credits(pool.remaining) + ' ' + pool.unit }}</p>
          <details v-if="pool.status === 'estimated' || pool.excluded_credits > 0" class="mt-2 text-micro text-faint"><summary class="cursor-pointer">计算依据</summary><template v-if="pool.status === 'estimated'"><p class="mt-1">近 {{ pool.window_days }} 天 · {{ pool.samples }} 次付费调用 · {{ pool.scope === 'account' ? '本账号' : '同类计费资源' }}</p><p class="mt-1">每{{ pool.unit }}约 {{ abbr(pool.tokens_per_unit || 0) }} Tokens</p></template><p v-if="pool.excluded_credits > 0" class="mt-1">未计入过期或适用范围未确认额度 {{ credits(pool.excluded_credits) }}</p></details>
          <p v-if="pool.status !== 'estimated'" class="mt-1 text-faint">{{ pool.reason?.replace('（至少5次、1000 Tokens）', '') }}</p>

          <p v-if="pool.stale || pool.deferred" class="mt-1 text-warn">{{ [pool.stale ? '余额待刷新，预测可能滞后' : '', pool.deferred ? '账号暂时冷却，恢复前不可调用' : ''].filter(Boolean).join('；') }}</p>


        </div>
      </article>
      </div>
      <p v-if="!prediction?.providers?.length" class="py-6 text-center text-small text-faint">暂无续航明细，请等待概览数据刷新。</p>
      <details class="mt-3 text-micro text-faint"><summary class="cursor-pointer">估算说明</summary><p class="mt-2">已覆盖 {{ prediction?.estimated_pools ?? 0 }}/{{ prediction?.total_pools ?? 0 }} 个账号计费资源。至少 5 次、1,000 Tokens 的有效付费样本才输出预测；优先近 7 天，样本不足参考近 30 天或同类计费资源。模型、缓存、价格与到期时间会影响实际续航。</p></details>
  </WModal>
</template>
