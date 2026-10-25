<script lang="ts">
import { ref as sharedRef } from 'vue'

export interface ModelPromotion {
  label: string
  description?: string
  region: string
  partial?: boolean
  valid_from?: string
  valid_until?: string
  timezone?: string
  daily?: { start: string; end: string }[]
  weekly?: { weekdays: number[]; start: string; end: string }[]
  trial_days?: number
}

// One minute clock and one open panel for the entire catalog, not per card.
const clock = sharedRef(Date.now())
let users = 0
let ticker: ReturnType<typeof setInterval> | undefined
let closeActive: (() => void) | undefined
</script>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'

const props = defineProps<{ promotions: ModelPromotion[] }>()
const visible = computed(() => props.promotions.filter(p => {
  if (!p.label) return false
  // Enrollment closing does not end a previously activated personal trial.
  if (p.trial_days) return true
  return (!p.valid_from || Date.parse(p.valid_from) <= clock.value)
    && (!p.valid_until || clock.value < Date.parse(p.valid_until))
}))
const labels = computed(() => [...new Set(visible.value.map(p => p.label))])
const region = (s: string) => s === 'domestic' ? '国内' : s === 'international' ? '国际' : s === 'cn' ? 'Trae IDE' : s === 'solo' ? 'Work/SOLO' : s
function weekdays(days: number[]) {
  const values = [...new Set(days)].filter(d => d >= 1 && d <= 7).sort((a, b) => a - b)
  const key = values.join(',')
  if (key === '1,2,3,4,5') return '周一至周五'
  if (key === '6,7') return '周六、周日'
  if (key === '1,2,3,4,5,6,7') return '每天'
  return values.map(d => ['周一', '周二', '周三', '周四', '周五', '周六', '周日'][d - 1]).join('、')
}
function timestamp(value: string, timezone?: string) {
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      timeZone: timezone || 'UTC', year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    }).format(new Date(value))
  } catch { return value }
}
const id = useId(), open = ref(false)
const button = ref<HTMLElement | null>(null), panel = ref<HTMLElement | null>(null)
const position = ref({ left: '0px', top: '0px' })
let leaveTimer: ReturnType<typeof setTimeout> | undefined
function cancelLeave() { clearTimeout(leaveTimer); leaveTimer = undefined }
function hide() {
  cancelLeave()
  open.value = false
  if (closeActive === hide) closeActive = undefined
  document.removeEventListener('pointerdown', outside, true)
  document.removeEventListener('keydown', key)
  window.removeEventListener('resize', hide)
  window.removeEventListener('scroll', scroll, true)
  window.removeEventListener('blur', hide)
}
function outside(e: PointerEvent) {
  if (!button.value?.contains(e.target as Node) && !panel.value?.contains(e.target as Node)) hide()
}
function key(e: KeyboardEvent) { if (e.key === 'Escape') hide() }
function scroll(e: Event) {
  if (!(e.target instanceof Node) || !panel.value?.contains(e.target)) hide()
}
function leave() { cancelLeave(); leaveTimer = setTimeout(hide, 120) }
async function show() {
  cancelLeave()
  clock.value = Date.now()
  if (open.value || !visible.value.length) return
  closeActive?.()
  closeActive = hide
  open.value = true
  await nextTick()
  if (!open.value || !button.value || !panel.value) return
  const anchor = button.value.getBoundingClientRect(), box = panel.value.getBoundingClientRect()
  const left = Math.max(8, Math.min(anchor.right - box.width, window.innerWidth - box.width - 8))
  const below = anchor.bottom + 8
  const top = below + box.height <= window.innerHeight - 8 ? below : Math.max(8, anchor.top - box.height - 8)
  position.value = { left: `${left}px`, top: `${top}px` }
  document.addEventListener('pointerdown', outside, true)
  document.addEventListener('keydown', key)
  window.addEventListener('resize', hide)
  window.addEventListener('scroll', scroll, true)
  window.addEventListener('blur', hide)
}
watch(() => props.promotions, hide)
watch(() => visible.value.length, n => { if (!n) hide() })
onMounted(() => {
  clock.value = Date.now()
  if (++users === 1) ticker = setInterval(() => { clock.value = Date.now() }, 60_000)
})
onUnmounted(() => {
  hide()
  if (--users === 0) { clearInterval(ticker); ticker = undefined }
})
</script>

<template>
  <button v-if="labels.length" ref="button" type="button"
    class="inline-flex max-w-full items-center gap-1 rounded border border-warn/25 bg-warn/5 px-1.5 py-0.5 font-sans text-micro font-normal leading-tight text-warn focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand"
    :aria-expanded="open" :aria-controls="id" aria-haspopup="dialog" aria-label="查看模型活动规则"
    @mouseenter="show" @mouseleave="leave" @focus="show" @blur="leave" @click="show">
    <span class="truncate">{{ labels[0] }}</span><span v-if="labels.length > 1" class="shrink-0">+{{ labels.length - 1 }}</span>
  </button>
  <Teleport to="body">
    <div v-if="open" :id="id" ref="panel" role="dialog" aria-label="模型活动规则" :style="position"
      class="fixed z-[90] max-h-[calc(100vh-1rem)] w-80 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-xl border border-line bg-surface p-4 text-left font-sans text-small font-normal text-ink shadow-xl"
      @mouseenter="cancelLeave" @mouseleave="leave">
      <div class="mb-3 font-medium">上游活动规则</div>
      <div class="space-y-3">
        <div v-for="(p, i) in visible" :key="i" class="border-t border-line pt-3">
          <div class="mb-1 flex flex-wrap items-center gap-x-2 gap-y-1">
            <span class="text-warn">{{ p.label }}</span>
            <span class="text-micro text-muted">{{ region(p.region) }}<template v-if="p.partial"> · 部分账号适用</template></span>
          </div>
          <p v-if="p.description" class="whitespace-pre-line break-words leading-relaxed">{{ p.description }}</p>
          <p v-if="p.trial_days" class="mt-1 text-micro text-muted">启用后 {{ p.trial_days }} 天试用，剩余时间以上游账号权益为准。</p>
          <div v-if="p.valid_from || p.valid_until" class="mt-2 space-y-1 text-micro text-muted">
            <div v-if="p.valid_from">{{ p.trial_days ? '开放领取' : '开始' }}：{{ timestamp(p.valid_from, p.timezone) }}</div>
            <div v-if="p.valid_until">{{ p.trial_days ? '领取截止' : '截止' }}：{{ timestamp(p.valid_until, p.timezone) }}（不含）</div>
          </div>
          <!-- Preserve official prose when it disagrees with structured clocks. -->
          <p v-if="!p.description && p.daily?.length" class="mt-1 text-micro text-muted">每日 {{ p.daily.map(w => `${w.start}–${w.end}`).join('、') }}</p>
          <div v-if="p.weekly?.length" class="mt-2 space-y-1 text-micro text-muted">
            <div v-for="(w, wi) in p.weekly" :key="wi">{{ weekdays(w.weekdays) }} {{ w.start === '00:00' && w.end === '24:00' ? '全天' : `${w.start}–${w.end}` }}</div>
          </div>
          <p v-if="p.timezone" class="mt-1 text-micro text-faint">时区：{{ p.timezone }}</p>
        </div>
      </div>
    </div>
  </Teleport>
</template>
