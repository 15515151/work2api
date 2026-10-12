<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { api } from '@/api/client'
import type { UpdateStatus, UpdatePolicy } from '@/types'
import { toast } from '@/lib/toast'
import WCard from './ui/WCard.vue'
import WButton from './ui/WButton.vue'
import WInput from './ui/WInput.vue'
import WToggle from './ui/WToggle.vue'
const status=ref<UpdateStatus|null>(null),error=ref(''),busy=ref(false),migration=ref(false),policyDirty=ref(false)
const policy=reactive<UpdatePolicy>({mode:'off',every_hours:6,start_hour:2,end_hour:4,allow_republished:false})
let disposed=false,timer:ReturnType<typeof setInterval>|undefined,loading=false
const phaseLabel=computed(()=>({idle:'未检查',checking:'检查中',checked:'检查完成',downloading:'下载校验中',ready:'待安装',installing:'准备安装',backup_ready:'备份完成',verifying:'新版验证中',accepting:'准备开放',acceptance_unknown:'需要核实开放状态',complete:'更新完成',rolled_back:'已恢复原版本',recovery_required:'需要人工恢复',failed:'更新失败',current:'已是相同产物'} as Record<string,string>)[status.value?.phase || ''] || status.value?.phase)
const inProgress=computed(()=>['checking','downloading','installing','backup_ready','verifying','accepting'].includes(status.value?.phase || ''))
const recoveryBlocked=computed(()=>['acceptance_unknown','recovery_required'].includes(status.value?.phase || ''))
const deploymentLabel=computed(()=>({source:'源码自编译',binary:'Release 二进制',systemd:'systemd 服务',docker:'Docker 容器'} as Record<string,string>)[status.value?.deployment.kind || ''] || '未识别')
async function load(initial=false){if(loading)return;loading=true;try{const result=await api.updateStatus();if(disposed)return;status.value=result;error.value='';if(initial||!policyDirty.value)Object.assign(policy,result.policy)}catch(e:any){if(!disposed)error.value=e?.response?.data?.error?.message || '无法读取更新状态；更新期间服务可能暂时重启，请稍后重试或重新登录'}finally{loading=false}}
async function action(kind:'check'|'download'|'install'|'policy'){
 if(busy.value || inProgress.value)return
 if(kind==='install'&&!confirm('安装将等待现有调用结束，备份后切换版本。期间暂停接收新请求，管理登录可能失效。是否继续？'))return
 busy.value=true;error.value=''
 try{
  if(kind==='check')await api.updateCheck()
  if(kind==='download')await api.updateDownload()
  if(kind==='install'){await api.updateInstall(migration.value,status.value?.candidate?.digest || '');toast.success('已提交安装，请等待更新助手完成；重启后可能需要重新登录')}
  if(kind==='policy'){await api.updatePolicy({...policy,every_hours:Number(policy.every_hours),start_hour:Number(policy.start_hour),end_hour:Number(policy.end_hour)});policyDirty.value=false;toast.success('更新策略已保存')}
  await load()
 }catch(e:any){error.value=e?.response?.data?.error?.message || '更新操作失败，请查看状态后重试'}finally{busy.value=false}
}
onMounted(()=>{load(true);timer=setInterval(()=>{load()},5000)})
onUnmounted(()=>{disposed=true;if(timer)clearInterval(timer)})
</script>
<template>
 <WCard title="版本更新">
  <p v-if="error" class="mb-3 whitespace-pre-wrap text-small text-fault">{{ error }} <button class="text-brand" @click="load()">重试</button></p>
  <template v-if="status">
   <div class="grid gap-3 sm:grid-cols-3 text-small"><div><div class="text-micro text-faint">当前版本 / 构建</div><div class="mono break-all">{{ status.current.version }}</div><div class="text-micro text-faint">{{ deploymentLabel }} · {{ status.current.os }}/{{ status.current.arch }}</div></div><div><div class="text-micro text-faint">最新稳定版</div><div class="mono">{{ status.latest?.tag_name || '未检查' }}</div><p v-if="status.same_version" class="text-micro text-faint">同版本可下载核对重发产物指纹</p></div><div><div class="text-micro text-faint">更新状态</div><div>{{ phaseLabel }}</div><p class="text-micro text-faint">{{ status.last_check ? '检查于 '+new Date(status.last_check).toLocaleString() : '尚未检查' }}</p></div></div>
   <p v-if="status.message" class="mt-3 break-words text-small text-muted">{{ status.message }}</p>
   <p v-if="!status.deployment.can_install" class="mt-3 rounded-lg border border-line bg-elevated p-3 text-small text-muted">{{ status.deployment.reason }}</p>
   <div class="mt-4 flex flex-wrap gap-2"><WButton size="sm" :disabled="busy || inProgress" @click="action('check')">检查更新</WButton><WButton size="sm" :disabled="busy || inProgress || !status.deployment.can_install || recoveryBlocked || !status.available" @click="action('download')">下载并校验</WButton><WButton size="sm" variant="primary" :disabled="busy || inProgress || !status.deployment.can_install || recoveryBlocked || !status.candidate || status.candidate.migration && !migration" @click="action('install')">安装已验证版本</WButton></div>
   <div v-if="status.candidate" class="mt-3 rounded-lg border border-line p-3 text-micro text-faint"><div class="mono break-all">SHA-256：{{ status.candidate.digest }}</div><label v-if="status.candidate.migration" class="mt-3 flex items-start gap-2"><WToggle v-model="migration" aria-label="确认数据库迁移" /><span>确认数据库迁移：更新前完整备份，失败时恢复更新前程序与数据；此类更新不自动安装。</span></label></div>
   <details v-if="status.latest?.body" class="mt-4 rounded-lg border border-line p-3"><summary class="cursor-pointer text-small text-muted">发行说明</summary><pre class="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-words font-sans text-small text-faint">{{ status.latest.body }}</pre></details>
   <div class="mt-5 border-t border-line pt-4" @input="policyDirty=true" @change="policyDirty=true">
    <h4 class="text-small font-medium">定时策略</h4><div class="mt-3 flex flex-wrap items-end gap-3"><label class="text-micro text-faint">更新模式<select v-model="policy.mode" class="mt-1 block rounded-lg border border-line bg-bg p-2 text-small text-ink"><option value="off">关闭（默认）</option><option value="check">定时检查，仅提示</option><option value="auto" :disabled="!status.deployment.can_install">维护时段自动安装</option></select></label><WInput v-model="policy.every_hours" type="number" label="检查间隔（小时）" class="w-40" /><WInput v-model="policy.start_hour" type="number" label="维护开始（0–23）" class="w-36" /><WInput v-model="policy.end_hour" type="number" label="维护结束（0–23）" class="w-36" /></div>
    <label class="mt-3 flex items-center gap-2 text-small text-muted"><WToggle v-model="policy.allow_republished" aria-label="自动安装同版本重发产物" @update:model-value="policyDirty=true" /><span>自动安装同版本重发产物（默认关闭）</span></label>
    <p class="mt-3 text-micro leading-relaxed text-faint">维护时段使用服务时区 {{ status.timezone }}，可跨午夜。自动更新仅安装稳定版且数据库版本不变；迁移需手动确认。失败后至少等待一小时，同一产物每天最多自动尝试一次。下载校验失败或等待调用结束超时，保留旧服务；每次更新保留必要恢复副本。</p>
    <WButton size="sm" class="mt-3" :disabled="busy || inProgress" @click="action('policy')">保存更新策略</WButton>
   </div>
  </template>
  <p v-else class="text-small text-faint">读取更新能力…</p>
 </WCard>
</template>
