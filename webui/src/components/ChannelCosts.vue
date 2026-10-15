<script lang="ts">
// One active cost popover; document listeners exist only while it is open.
let closeActive: (() => void) | undefined
</script>
<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, useId, watch } from 'vue'
import { credits } from '@/lib/format'
import { providerMeta } from '@/lib/providers'

const props=defineProps<{total?:number|null;costs?:{provider:string;credits:number|null;partial?:boolean}[]}>()
const total=computed(()=>props.total ?? (props.costs?.some(c=>c.credits!=null) ? props.costs.reduce((sum,c)=>sum+(c.credits ?? 0),0) : null))
const label=(provider:string)=>provider==='unknown'?'未确认渠道':providerMeta(provider).label
const mixed=computed(()=>new Set(props.costs?.map(c=>c.provider)).size>1)
const partial=computed(()=>props.costs?.some(c=>c.partial || c.credits==null))
const id=useId(),open=ref(false),button=ref<HTMLElement|null>(null),panel=ref<HTMLElement|null>(null)
const position=ref({left:'0px',top:'0px'})
let leaveTimer:ReturnType<typeof setTimeout>|undefined
function cancelLeave(){if(leaveTimer){clearTimeout(leaveTimer);leaveTimer=undefined}}
function hide(){
 cancelLeave();open.value=false
 if(closeActive===hide)closeActive=undefined
 document.removeEventListener('pointerdown',outside,true)
 document.removeEventListener('keydown',key)
 window.removeEventListener('resize',hide)
 window.removeEventListener('scroll',move,true)
}
function outside(e:PointerEvent){const target=e.target as Node;if(!button.value?.contains(target)&&!panel.value?.contains(target))hide()}
function key(e:KeyboardEvent){if(e.key==='Escape')hide()}
function move(e:Event){if(e.target instanceof Node && panel.value?.contains(e.target))return;hide()}
function leave(){cancelLeave();leaveTimer=setTimeout(hide,120)}
async function show(){
 cancelLeave()
 if(open.value)return
 closeActive?.();closeActive=hide;open.value=true
 await nextTick()
 if(!open.value || !button.value || !panel.value)return
 const anchor=button.value.getBoundingClientRect(),box=panel.value.getBoundingClientRect()
 const left=Math.max(8,Math.min(anchor.right-box.width,window.innerWidth-box.width-8))
 const below=anchor.bottom+8
 const top=below+box.height<=window.innerHeight-8?below:Math.max(8,anchor.top-box.height-8)
 position.value={left:`${left}px`,top:`${top}px`}
 document.addEventListener('pointerdown',outside,true)
 document.addEventListener('keydown',key)
 window.addEventListener('resize',hide)
 window.addEventListener('scroll',move,true)
}
watch(()=>[props.total,props.costs],hide)
onUnmounted(hide)
</script>
<template>
 <button ref="button" type="button" class="mono whitespace-nowrap rounded text-small text-ink decoration-dotted underline-offset-4 hover:text-brand hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand" :aria-expanded="open" :aria-controls="id" aria-haspopup="dialog" aria-label="查看累计消耗明细" @mouseenter="show" @mouseleave="leave" @focus="show" @blur="leave" @click="show">{{credits(total)}}</button>
 <Teleport to="body"><div v-if="open" :id="id" ref="panel" role="dialog" aria-label="累计消耗明细" :style="position" class="fixed z-[90] w-64 max-w-[calc(100vw-1rem)] max-h-[calc(100vh-1rem)] overflow-y-auto rounded-xl border border-line bg-surface p-4 text-small text-ink shadow-xl" @mouseenter="cancelLeave" @mouseleave="leave">
  <div class="mb-3 flex justify-between gap-3 font-medium"><span>累计消耗</span><span class="mono">{{credits(total)}}</span></div>
  <dl v-if="costs?.length" class="space-y-2 border-t border-line pt-3"><div v-for="cost in costs" :key="cost.provider" class="flex justify-between gap-3"><dt class="text-muted">{{label(cost.provider)}}</dt><dd class="mono text-right">{{credits(cost.credits)}}<span class="ml-1 font-sans text-micro text-faint">{{['workbuddy','qoder','trae'].includes(cost.provider)?'积分':'上游单位'}}</span></dd></div></dl>
  <p v-else class="text-micro text-faint">暂无渠道消耗记录。</p>
  <p v-if="mixed" class="mt-3 text-micro leading-relaxed text-muted">合计为各渠道已记录数值之和，单位不统一，不能视为统一余额或金额。</p>
  <p v-if="partial" class="mt-2 text-micro text-muted">部分调用未上报消耗，合计仅包含已记录值。</p>
 </div></Teleport>
</template>
