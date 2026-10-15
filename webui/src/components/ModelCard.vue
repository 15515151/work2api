<script setup lang="ts">
import { computed, ref } from 'vue'
import ModelInfoCard from './ModelInfoCard.vue'
import WButton from './ui/WButton.vue'
import WIcon from './ui/WIcon.vue'
import { api } from '@/api/client'
import { toast } from '@/lib/toast'
const props = defineProps<{ model: Record<string, any>; provider: string }>()
const m = computed(() => props.model)
const accounts = computed(() => (m.value.accounts || []) as any[])
const systemOne = computed(() => String(m.value.native_protocol || '').toLowerCase() === 'systemone')
// ---- 一键测试（单模型单次）----
// 不做批量：一次只测一个模型，prompt 可编辑（默认 hi），max_tokens 上限 64。
// 后端有 5s 最小间隔 + 单飞频控。SystemOne 决策模型不支持聊天调用，不给按钮。
interface TestResult {
  ok: boolean
  latency_ms: number
  content?: string
  error?: string
  finish_reason?: string
}
const testResult = ref<TestResult | null>(null)
const testing = ref(false)
const testPrompt = ref('')

async function runTest() {
  if (testing.value || systemOne.value) return
  testing.value = true
  testResult.value = null
  try {
    const res: TestResult = await api.modelTest(m.value.id, testPrompt.value || 'hi')
    testResult.value = res
    if (!res.ok) toast.error(`测试失败：${res.error || '未知错误'}`)
  } catch (e: any) {
    testResult.value = { ok: false, latency_ms: 0, error: e?.message || String(e) }
    toast.error('测试请求失败')
  } finally {
    testing.value = false
  }
}

</script>
<template>
  <ModelInfoCard v-bind="props">
    <template #actions>
      <!-- 一键测试：SystemOne 模型不支持聊天调用，不提供按钮。结果显示在下方。 -->
      <div v-if="!systemOne" class="border-t border-line pt-2.5">
        <div class="flex items-center gap-2">
          <WButton size="sm" variant="ghost" :loading="testing" @click="runTest">
            <WIcon v-if="!testing" name="bolt" :size="14" /> 测试
          </WButton>
          <input
            v-model="testPrompt"
            type="text"
            placeholder="hi（可编辑测试语）"
            class="mono h-8 min-w-0 flex-1 rounded-lg border border-line bg-bg/40 px-2 text-micro text-ink placeholder:text-faint focus:border-brand focus:outline-none"
            @keydown.enter="runTest"
          />
        </div>
        <div v-if="testResult" class="mt-2 rounded-lg border px-2.5 py-1.5 text-micro leading-relaxed"
          :class="testResult.ok ? 'border-live/30 bg-live/5 text-muted' : 'border-fault/30 bg-fault/5 text-fault'">
          <span class="mono">{{ testResult.latency_ms }}ms</span>
          <template v-if="testResult.ok"> · <span class="text-live">通过</span><template v-if="testResult.finish_reason"> · {{ testResult.finish_reason }}</template> · <span>{{ testResult.content }}</span></template>
          <template v-else> · <span>{{ testResult.error || '失败' }}</span></template>
        </div>
      </div>
    </template>
    <template #footer>
      <!-- Account rows start at the same shared track, including wrapped labels. -->
      <div v-if="accounts.length" class="flex flex-wrap items-center gap-1.5 border-t border-line pt-2.5">
        <span class="text-micro text-faint">可用账号</span>
        <span
          v-for="a in accounts.slice(0, 6)"
          :key="a.uid"
          class="inline-flex max-w-full items-center gap-1 rounded-md bg-elevated px-1.5 py-0.5 text-micro"
          :class="a.healthy ? 'text-live' : 'text-faint'"
          :title="`${a.label || a.uid}${a.site_label ? ' · ' + a.site_label : ''}${a.model_cooldown ? ' · 本模型冷却中' : ''}`"
        >
          <span class="h-1.5 w-1.5 rounded-full" :class="a.healthy ? 'bg-live' : 'bg-faint'" />
          <span class="truncate">{{ a.label || a.uid.slice(0, 6) }}</span>
        </span>
        <span v-if="accounts.length > 6" class="text-micro text-faint">+{{ accounts.length - 6 }}</span>
      </div>
    </template>
  </ModelInfoCard>
</template>
