<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" :aria-busy="loading">
    <h3 class="font-semibold">{{ t('admin.settings.systemTimezone.title') }}</h3>
    <p class="text-sm text-gray-500">{{ t('admin.settings.systemTimezone.description') }}</p>
    <template v-if="settings">
      <p class="text-sm">{{ t('admin.settings.systemTimezone.effective', { timezone: settings.effective_timezone }) }}</p>
      <label for="system-timezone" class="block text-sm font-medium">{{ t('admin.settings.systemTimezone.label') }}</label>
      <input id="system-timezone" v-model="value" type="text" list="system-timezones" class="input" placeholder="Asia/Taipei" :disabled="loading" />
      <datalist id="system-timezones"><option v-for="zone in zones" :key="zone" :value="zone" /></datalist>
      <p class="text-xs text-gray-500">{{ t('admin.settings.systemTimezone.fallback', { timezone: settings.fallback_timezone }) }}</p>
      <p v-if="settings.restart_required" role="status" class="text-sm text-amber-600">{{ t('admin.settings.systemTimezone.pending') }}</p>
      <button type="button" class="btn btn-primary btn-sm" :disabled="loading" @click="save">{{ t('admin.settings.systemTimezone.save') }}</button>
    </template>
    <button v-else type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('admin.settings.systemTimezone.retry') }}</button>
    <p v-if="error" role="alert" class="text-sm text-red-500">{{ error }}</p>
    <p v-if="saved" role="status" class="text-sm text-green-600">{{ t('admin.settings.systemTimezone.saved') }}</p>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { systemTimezoneAPI, type SystemTimezoneSettings } from '@/api/admin/systemTimezone'

const { t } = useI18n()
const settings = ref<SystemTimezoneSettings | null>(null)
const value = ref('')
const loading = ref(false)
const error = ref('')
const saved = ref(false)
const zones = ['Asia/Taipei', 'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Tokyo', 'Asia/Singapore', 'America/Los_Angeles', 'America/New_York', 'Europe/London', 'UTC']

async function load() {
  loading.value = true
  error.value = ''
  try { settings.value = await systemTimezoneAPI.get(); value.value = settings.value.timezone }
  catch { error.value = t('admin.settings.systemTimezone.error') }
  finally { loading.value = false }
}
async function save() {
  loading.value = true
  error.value = ''
  saved.value = false
  try { settings.value = await systemTimezoneAPI.save(value.value.trim()); value.value = settings.value.timezone; saved.value = true }
  catch { error.value = t('admin.settings.systemTimezone.error') }
  finally { loading.value = false }
}
onMounted(load)
</script>
