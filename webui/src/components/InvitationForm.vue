<script setup lang="ts">
import { ref } from 'vue'
import WInput from './ui/WInput.vue'
import WButton from './ui/WButton.vue'
const props = defineProps<{ initial?: {max_uses: number; expires_at: number; used_count: number; enabled: number}; disabled?: boolean }>()
const emit = defineEmits<{ save: [value: {max_uses: number; expires_at: number; enabled: boolean}] }>()
const unlimited = ref(props.initial?.max_uses === 0)
const maxUses = ref(String(props.initial?.max_uses || 1))
const expiryMode = ref(props.initial ? props.initial.expires_at ? 'custom' : 'never' : '7')
function localDate(ts: number) { if (!ts) return ''; const d = new Date(ts * 1000); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0,16) }
const date = ref(localDate(props.initial?.expires_at || 0))
const enabled = ref(props.initial ? !!props.initial.enabled : true)
const error = ref('')
function save() {
 if (props.disabled) return
 error.value = ''
 const count = unlimited.value ? 0 : Number(maxUses.value)
 if (!unlimited.value && (!maxUses.value.trim() || !Number.isSafeInteger(count) || count < 1)) { error.value = '使用次数需为正整数，或选择不限次数'; return }
 if (count && count < (props.initial?.used_count || 0)) { error.value = '次数上限不能低于已经使用的次数'; return }
 const expires = expiryMode.value === 'never' ? 0 : expiryMode.value === 'custom' ? props.initial && date.value === localDate(props.initial.expires_at) ? props.initial.expires_at : new Date(date.value).getTime()/1000 : Math.floor(Date.now()/1000) + Number(expiryMode.value)*86400
 if (!Number.isFinite(expires) || (expiryMode.value === 'custom' && !date.value) || expires < 0 || expires > 253402300799 || (!props.initial && expires !== 0 && expires <= Date.now()/1000)) { error.value = '请选择有效的到期时间；新邀请码的到期时间须晚于当前时间'; return }
 emit('save', {max_uses: count, expires_at: expires, enabled: enabled.value})
}
</script>
<template>
 <div class="space-y-4">
  <div class="flex flex-wrap items-end gap-3">
   <div class="min-w-0 flex-1"><span class="mb-1 block text-small text-muted">可使用次数</span><WInput v-model="maxUses" type="number" aria-label="邀请码可使用次数" :disabled="disabled || unlimited" placeholder="1" /></div>
   <label class="flex h-9 items-center gap-2 text-small"><input v-model="unlimited" type="checkbox" :disabled="disabled">不限次数</label>
  </div>
  <div class="flex flex-wrap items-end gap-3">
   <div class="min-w-0 flex-1"><span class="mb-1 block text-small text-muted">有效期</span><select v-model="expiryMode" :disabled="disabled" aria-label="邀请码有效期" class="h-9 w-full rounded-lg border border-line bg-bg px-3 text-small"><option value="1">1 天</option><option value="7">7 天</option><option value="30">30 天</option><option value="90">90 天</option><option value="custom">指定到期时间</option><option value="never">永不过期</option></select></div>
   <div v-if="expiryMode === 'custom'" class="min-w-0 flex-1"><span class="mb-1 block text-small text-muted">本地时间</span><WInput v-model="date" type="datetime-local" aria-label="邀请码到期时间" :disabled="disabled" /></div>
  </div>
  <label v-if="initial" class="flex items-center gap-2 text-small"><input v-model="enabled" type="checkbox" :disabled="disabled">启用邀请码（停用不影响已注册用户）</label>
  <p v-if="error" role="alert" class="text-small text-fault">{{ error }}</p>
  <WButton :disabled="disabled" @click="save">{{ initial ? '保存配置' : '生成邀请码' }}</WButton>
 </div>
</template>
