<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ selected: string[]; results: string[]; selectable?: string[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:selected': [value: string[]] }>()
const eligible = computed(() => [...new Set(props.results)].filter(m => !props.selectable || props.selectable.includes(m)))
const selectedResults = computed(() => props.selected.filter(m => props.results.includes(m)))
const allSelected = computed(() => eligible.value.length > 0 && eligible.value.every(m => props.selected.includes(m)))
function selectResults() { if (!props.disabled) emit('update:selected', [...new Set([...props.selected, ...eligible.value])]) }
function removeResults() { if (!props.disabled) emit('update:selected', props.selected.filter(m => !props.results.includes(m))) }
function clearAll() { if (!props.disabled) emit('update:selected', []) }
</script>
<template>
  <div class="flex flex-wrap items-center gap-3 text-micro">
    <button type="button" class="text-brand disabled:cursor-not-allowed disabled:opacity-40" :disabled="disabled || !eligible.length || allSelected" @click="selectResults">全选当前结果（{{ eligible.length }}）</button>
    <button type="button" class="text-brand disabled:cursor-not-allowed disabled:opacity-40" :disabled="disabled || !selectedResults.length" @click="removeResults">取消当前结果</button>
    <button type="button" class="text-muted disabled:cursor-not-allowed disabled:opacity-40" :disabled="disabled || !selected.length" @click="clearAll">清空全部</button>
    <span class="text-faint">已选 {{ selected.length }} 项</span>
  </div>
</template>
