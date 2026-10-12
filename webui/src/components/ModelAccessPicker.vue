<script setup lang="ts">
import { computed, ref } from 'vue'
import WInput from './ui/WInput.vue'
import ModelSelectionActions from './ModelSelectionActions.vue'
const props = defineProps<{ options: string[]; modelValue: string[]; disabled?: boolean; names?: Record<string, string> }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const search = ref('')
const visible = computed(() => [...new Set([...props.options, ...props.modelValue])].sort().filter(m => `${m} ${props.names?.[m] || ''}`.toLowerCase().includes(search.value.trim().toLowerCase())))
function toggle(model: string, checked: boolean) { emit('update:modelValue', checked ? [...new Set([...props.modelValue, model])] : props.modelValue.filter(m => m !== model)) }
</script>
<template>
  <div class="space-y-3">
    <WInput v-model="search" placeholder="搜索模型名称或 ID" />
    <ModelSelectionActions :selected="modelValue" :results="visible" :selectable="options" :disabled="disabled" @update:selected="emit('update:modelValue', $event)" />
    <div class="grid max-h-64 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
      <label v-for="m in visible" :key="m" class="flex min-w-0 items-start gap-2 rounded-lg border border-line p-2 text-small">
        <input type="checkbox" :checked="modelValue.includes(m)" :disabled="disabled" class="mt-1 shrink-0" @change="toggle(m, ($event.target as HTMLInputElement).checked)" />
        <span class="min-w-0 break-words"><span class="block">{{ names?.[m] || m }}</span><code v-if="names?.[m] && names[m] !== m" class="block break-all text-micro text-faint">{{ m }}</code><span v-if="!options.includes(m)" class="text-micro text-warn">当前不在目录，配置保留</span></span>
      </label>
    </div>
    <p v-if="!visible.length" class="text-small text-faint">暂无匹配模型，可刷新目录后重试。</p>
  </div>
</template>
