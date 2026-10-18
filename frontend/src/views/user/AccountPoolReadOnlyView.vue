<template>
  <AppLayout>
    <div class="space-y-6">
      <div><h1 class="text-2xl font-semibold">{{ t('accountViewer.title') }}</h1><p class="mt-1 text-sm text-gray-500">{{ t('accountViewer.description') }}</p></div>
      <form class="card flex flex-wrap gap-3 p-4" @submit.prevent="page = 1; loadAccounts()">
        <button type="submit" class="btn btn-primary" :disabled="loading">{{ t('accountViewer.refresh') }}</button>
      </form>
      <p v-if="error" role="alert" class="text-red-500">{{ error }}</p>
      <div class="card overflow-x-auto" :aria-busy="loading">
        <p v-if="loading" role="status" class="p-6 text-gray-500">{{ t('accountViewer.loading') }}</p>
        <p v-else-if="!accounts.length" class="p-6 text-gray-500">{{ t('accountViewer.empty') }}</p>
        <table v-else class="w-full text-left text-sm">
          <thead class="border-b border-gray-200 dark:border-dark-600"><tr><th v-for="key in ['account', 'pool', 'status', 'quota', 'stats']" :key="key" class="p-4">{{ t(`accountViewer.${key}`) }}</th></tr></thead>
          <tbody><tr v-for="account in accounts" :key="account.id" class="border-b border-gray-100 dark:border-dark-700">
            <td class="p-4"><div class="font-medium">{{ account.name || `#${account.id}` }}</div><div class="mt-1 text-xs text-gray-500">#{{ account.id }} · {{ account.platform }} · {{ account.type }}</div></td>
            <td class="p-4">{{ account.groups.map(group => group.name || `#${group.id}`).join('、') || '—' }}</td>
            <td class="p-4"><span class="badge badge-gray">{{ statusLabel(account) }}</span></td>
            <td class="min-w-48 p-4">
              <div v-for="window in account.usage_windows.filter(item => item.used_percent !== null)" :key="window.window" class="mb-2">
                <div>{{ window.label === '7d_oi' ? 'Fable 7d' : window.label }}: {{ window.used_percent?.toFixed(1) }}% <span v-if="window.status !== 'ok'" class="text-xs text-gray-500">· {{ t('accountViewer.stale') }}</span></div>
                <div v-if="window.observed_at" class="text-xs text-gray-500">{{ t('accountViewer.observed') }}: {{ dateTime(window.observed_at) }}</div>
                <div v-if="window.resets_at" class="text-xs text-gray-500">{{ t('accountViewer.resets') }}: {{ dateTime(window.resets_at) }}</div>
              </div>
              <span v-if="!account.usage_windows.some(item => item.used_percent !== null)" class="text-gray-500">{{ t('accountViewer.noSnapshot') }}</span>
            </td>
            <td class="p-4"><button class="btn btn-secondary btn-sm" type="button" @click="selectAccount(account)">{{ t('accountViewer.stats') }}</button></td>
          </tr></tbody>
        </table>
      </div>
      <div class="flex items-center justify-between gap-3 text-sm">
        <span>{{ t('accountViewer.total', { count: total }) }}</span>
        <div class="flex items-center gap-3"><button class="btn btn-secondary btn-sm" :disabled="page <= 1 || loading" @click="page--; loadAccounts()">{{ t('accountViewer.previous') }}</button><span>{{ page }} / {{ Math.max(1, pages) }}</span><button class="btn btn-secondary btn-sm" :disabled="page >= pages || loading" @click="page++; loadAccounts()">{{ t('accountViewer.next') }}</button></div>
      </div>
      <section v-if="selected" class="card space-y-4 p-4" :aria-busy="statsLoading">
        <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ selected.name || `#${selected.id}` }} · {{ t('accountViewer.title') }}</h2><select v-model.number="days" class="input w-auto" :aria-label="t('accountViewer.stats')" @change="loadStats()"><option v-for="value in [1, 7, 30, 90]" :key="value" :value="value">{{ t('accountViewer.period', { days: value }) }}</option></select></div>
        <p v-if="statsLoading" role="status">{{ t('accountViewer.loading') }}</p>
        <p v-if="statsError" role="alert" class="text-red-500">{{ statsError }}</p>
        <template v-if="stats">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3"><div v-for="key in ['requests', 'tokens', 'cost'] as const" :key="key" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800"><p class="text-sm text-gray-500">{{ t(`accountViewer.${key}`) }}</p><p class="mt-2 text-2xl font-semibold">{{ key === 'cost' ? stats.summary.cost.toFixed(4) : stats.summary[key].toLocaleString() }}</p></div></div>
          <div class="overflow-x-auto"><table v-if="stats.history.length" class="w-full text-left text-sm"><thead><tr><th v-for="key in ['date', 'requests', 'tokens', 'cost']" :key="key" class="p-3">{{ t(`accountViewer.${key}`) }}</th></tr></thead><tbody><tr v-for="day in stats.history" :key="day.date" class="border-t border-gray-100 dark:border-dark-700"><td class="p-3">{{ day.date }}</td><td class="p-3">{{ day.requests.toLocaleString() }}</td><td class="p-3">{{ day.tokens.toLocaleString() }}</td><td class="p-3">{{ day.cost.toFixed(4) }}</td></tr></tbody></table><p v-else class="text-gray-500">{{ t('accountViewer.noHistory') }}</p></div>
        </template>
      </section>
      <p class="text-xs text-gray-500">{{ t('accountViewer.statsHint') }}</p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { accountViewerAPI, type ViewerAccount, type ViewerStats } from '@/api/accountViewer'
const { t } = useI18n()
const accounts = ref<ViewerAccount[]>([])
const loading = ref(false), error = ref(''), page = ref(1), pages = ref(0), total = ref(0)
const selected = ref<ViewerAccount | null>(null), stats = ref<ViewerStats | null>(null)
const days = ref(30), statsLoading = ref(false), statsError = ref('')
let listController: AbortController | undefined, statsController: AbortController | undefined
function dateTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString() }
function statusLabel(account: ViewerAccount) {
  if (account.status === 'disabled') return t('accountViewer.disabled')
  if (account.status !== 'active') return t('accountViewer.accountError')
  if ([account.rate_limit_reset_at, account.overload_until, account.temp_unschedulable_until].some(value => value && new Date(value).getTime() > Date.now())) return t('accountViewer.limited')
  return t(account.schedulable ? 'accountViewer.active' : 'accountViewer.paused')
}
async function loadAccounts() {
  listController?.abort(); statsController?.abort()
  const controller = new AbortController(); listController = controller
  loading.value = true; error.value = ''; accounts.value = []; selected.value = null; stats.value = null
  try {
    const data = await accountViewerAPI.list({ page: page.value, page_size: 20 }, controller.signal)
    if (controller.signal.aborted) return
    accounts.value = data.items; total.value = data.total; pages.value = data.pages
  } catch { if (!controller.signal.aborted) error.value = t('accountViewer.error') }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function selectAccount(account: ViewerAccount) { selected.value = account; void loadStats() }
async function loadStats() {
  statsController?.abort()
  if (!selected.value) return
  const controller = new AbortController(); statsController = controller
  statsLoading.value = true; statsError.value = ''; stats.value = null
  try {
    const data = await accountViewerAPI.stats(selected.value.id, days.value, controller.signal)
    if (!controller.signal.aborted) stats.value = data
  } catch { if (!controller.signal.aborted) statsError.value = t('accountViewer.error') }
  finally { if (!controller.signal.aborted) statsLoading.value = false }
}
onMounted(loadAccounts)
onBeforeUnmount(() => { listController?.abort(); statsController?.abort() })
</script>
