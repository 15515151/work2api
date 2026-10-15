<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ selected: string[]; results: string[]; selectable?: string[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:selected': [value: string[]] }>()
const resultSet = computed(() => new Set(props.results))
const selectedSet = computed(() => new Set(props.selected))
const eligible = computed(() => { const selectable = props.selectable ? new Set(props.selectable) : null; return [...resultSet.value].filter(m => !selectable || selectable.has(m)) })
const selectedResults = computed(() => props.selected.filter(m => resultSet.value.has(m)))
const allSelected = computed(() => eligible.value.length > 0 && eligible.value.every(m => selectedSet.value.has(m)))
function selectResults() { if (!props.disabled) emit('update:selected', [...new Set([...props.selected, ...eligible.value])]) }
function removeResults() { if (!props.disabled) emit('update:selected', props.selected.filter(m => !resultSet.value.has(m))) }
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
