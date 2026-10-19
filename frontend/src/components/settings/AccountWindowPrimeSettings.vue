<template>
  <div class="border-t border-gray-100 pt-4 dark:border-dark-700" :aria-busy="loading" data-testid="window-prime-settings">
    <div class="mb-3">
      <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.windowPrime.title') }}</label>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.windowPrime.description') }}</p>
      <p class="mt-0.5 text-xs text-amber-600 dark:text-amber-400">{{ t('admin.settings.windowPrime.risk') }}</p>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.windowPrime.separateSave') }}</p>
    </div>

    <template v-if="form">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div
          v-for="platform in WINDOW_PRIME_PLATFORMS"
          :key="platform"
          class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
        >
          <div class="flex items-center justify-between gap-3">
            <div>
              <label class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ platform }}</label>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.windowPrime.platformEnabled') }}</p>
            </div>
            <Toggle v-model="form.platforms[platform].enabled" :data-testid="`window-prime-${platform}-enabled`" />
          </div>
          <label class="input-label mt-3">{{ t('admin.settings.windowPrime.model') }}</label>
          <WindowPrimeModelSelect
            v-model="form.platforms[platform].model"
            :options="modelOptions[platform] ?? []"
            :empty-label="t('admin.settings.windowPrime.modelDefault', { model: defaultModels[platform] ?? '' })"
            :test-id="`window-prime-${platform}-model`"
          />
        </div>
      </div>

      <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.settings.windowPrime.delay') }}</label>
          <div class="grid grid-cols-2 gap-3">
            <input
              v-model.number="form.delay_min_minutes"
              type="number"
              min="0"
              :max="MAX_WINDOW_PRIME_DELAY_MINUTES"
              class="input"
              :aria-label="t('admin.settings.windowPrime.delayMin')"
              data-testid="window-prime-delay-min"
            />
            <input
              v-model.number="form.delay_max_minutes"
              type="number"
              min="0"
              :max="MAX_WINDOW_PRIME_DELAY_MINUTES"
              class="input"
              :aria-label="t('admin.settings.windowPrime.delayMax')"
              data-testid="window-prime-delay-max"
            />
          </div>
          <p class="input-hint">{{ t('admin.settings.windowPrime.delayHint') }}</p>
        </div>
        <div>
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.settings.windowPrime.activeHours') }}</label>
            <Toggle v-model="form.active_hours_enabled" data-testid="window-prime-active-hours-enabled" />
          </div>
          <div v-if="form.active_hours_enabled" class="mt-2 grid grid-cols-2 gap-3">
            <input
              v-model="form.active_hours_start"
              type="text"
              class="input"
              placeholder="08:00"
              :aria-label="t('admin.settings.windowPrime.activeHoursStart')"
              data-testid="window-prime-active-hours-start"
            />
            <input
              v-model="form.active_hours_end"
              type="text"
              class="input"
              placeholder="24:00"
              :aria-label="t('admin.settings.windowPrime.activeHoursEnd')"
              data-testid="window-prime-active-hours-end"
            />
          </div>
          <p class="input-hint">{{ t('admin.settings.windowPrime.activeHoursHint') }}</p>
        </div>
      </div>

      <div class="mt-4">
        <label class="input-label">{{ t('admin.settings.windowPrime.prompts') }}</label>
        <textarea v-model="promptsText" rows="4" class="input" placeholder="Hi" data-testid="window-prime-prompts" />
        <p class="input-hint">{{ t('admin.settings.windowPrime.promptsHint') }}</p>
      </div>

      <button
        type="button"
        class="btn btn-primary btn-sm mt-4"
        :disabled="loading"
        data-testid="window-prime-save"
        @click="save"
      >
        {{ t('admin.settings.windowPrime.save') }}
      </button>
    </template>
    <button v-else type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
      {{ t('admin.settings.windowPrime.retry') }}
    </button>
    <p v-if="error" role="alert" class="mt-2 text-sm text-red-500">{{ error }}</p>
    <p v-if="saved" role="status" class="mt-2 text-sm text-green-600">{{ t('admin.settings.windowPrime.saved') }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import WindowPrimeModelSelect from './WindowPrimeModelSelect.vue'
import { accountWindowPrimeAPI, type AccountWindowPrimeSettings } from '@/api/admin/accountWindowPrime'
import {
  MAX_WINDOW_PRIME_DELAY_MINUTES,
  WINDOW_PRIME_PLATFORMS,
  isValidWindowPrimeClock,
  isValidWindowPrimeDelay,
  isValidWindowPrimePrompts,
  splitWindowPrimePrompts
} from '@/utils/accountWindowPrime'

const { t } = useI18n()
const form = ref<AccountWindowPrimeSettings | null>(null)
const defaultModels = ref<Record<string, string>>({})
const modelOptions = ref<Record<string, string[]>>({})
const promptsText = ref('')
const loading = ref(false)
const error = ref('')
const saved = ref(false)

function apply(settings: AccountWindowPrimeSettings) {
  const platforms: AccountWindowPrimeSettings['platforms'] = {}
  for (const platform of WINDOW_PRIME_PLATFORMS) {
    platforms[platform] = { enabled: settings.platforms?.[platform]?.enabled === true, model: settings.platforms?.[platform]?.model ?? '' }
  }
  defaultModels.value = settings.default_models ?? {}
  modelOptions.value = settings.model_options ?? {}
  form.value = { ...settings, platforms }
  promptsText.value = (settings.prompts ?? []).join('\n')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    apply(await accountWindowPrimeAPI.get())
  } catch {
    error.value = t('admin.settings.windowPrime.loadError')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value) return
  error.value = ''
  saved.value = false
  const prompts = splitWindowPrimePrompts(promptsText.value)
  const settings = { ...form.value, prompts }
  delete settings.default_models
  delete settings.model_options
  if (
    !isValidWindowPrimeDelay(settings.delay_min_minutes, settings.delay_max_minutes) ||
    !isValidWindowPrimeClock(settings.active_hours_start) ||
    !isValidWindowPrimeClock(settings.active_hours_end, true) ||
    !isValidWindowPrimePrompts(prompts)
  ) {
    error.value = t('admin.settings.windowPrime.invalid')
    return
  }
  loading.value = true
  try {
    apply(await accountWindowPrimeAPI.save(settings))
    saved.value = true
  } catch {
    error.value = t('admin.settings.windowPrime.error')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
