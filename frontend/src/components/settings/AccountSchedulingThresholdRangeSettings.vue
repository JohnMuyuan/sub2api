<template>
  <div class="border-t border-gray-100 pt-4 dark:border-dark-700" :aria-busy="loading" data-testid="scheduling-threshold-range-settings">
    <div class="mb-3">
      <label class="font-medium text-gray-900 dark:text-white">
        {{ t('admin.settings.schedulingThresholdRange.title') }}
      </label>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.schedulingThresholdRange.description') }}
      </p>
      <p class="mt-0.5 text-xs text-amber-600 dark:text-amber-400">
        {{ t('admin.settings.schedulingThresholdRange.separateSave') }}
      </p>
    </div>

    <template v-if="loaded">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <div
          v-for="platform in SCHEDULING_THRESHOLD_PLATFORMS"
          :key="platform"
          class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <label class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ platform }}</label>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{
                  ranges[platform].enabled
                    ? t('admin.settings.schedulingThresholdRange.enabled')
                    : t('admin.settings.schedulingThresholdRange.fixedFallback', { value: thresholds?.[platform] ?? 100 })
                }}
              </p>
            </div>
            <Toggle
              v-model="ranges[platform].enabled"
              :data-testid="`scheduling-threshold-range-${platform}-enabled`"
            />
          </div>
          <SchedulingThresholdRangeFields
            v-if="ranges[platform].enabled"
            v-model="ranges[platform].value"
            class="mt-3"
            :test-id-prefix="`scheduling-threshold-range-${platform}`"
          />
          <SchedulingThresholdWindowRules
            v-model="windowRules[platform]"
            class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700"
            :platform="platform"
            :test-id-prefix="`scheduling-threshold-window-${platform}`"
          />
        </div>
      </div>
      <button
        type="button"
        class="btn btn-primary btn-sm mt-4"
        :disabled="loading"
        data-testid="scheduling-threshold-range-save"
        @click="save"
      >
        {{ t('admin.settings.schedulingThresholdRange.save') }}
      </button>
    </template>
    <button v-else type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
      {{ t('admin.settings.schedulingThresholdRange.retry') }}
    </button>
    <p v-if="error" role="alert" class="mt-2 text-sm text-red-500">{{ error }}</p>
    <p v-if="saved" role="status" class="mt-2 text-sm text-green-600">
      {{ t('admin.settings.schedulingThresholdRange.saved') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import SchedulingThresholdRangeFields from './SchedulingThresholdRangeFields.vue'
import SchedulingThresholdWindowRules from './SchedulingThresholdWindowRules.vue'
import { SCHEDULING_THRESHOLD_PLATFORMS } from '@/api/admin/settings'
import {
  accountSchedulingThresholdRangesAPI,
  type AccountSchedulingThresholdCustomSettings,
  type AccountSchedulingThresholdRanges,
  type AccountSchedulingThresholdWindowRuleMap
} from '@/api/admin/accountSchedulingThresholdRanges'
import {
  DEFAULT_SCHEDULING_THRESHOLD_RANGE,
  isValidSchedulingThresholdRange,
  isValidSchedulingThresholdWindowRules,
  parseSchedulingThresholdWindowRules,
  type SchedulingThresholdRangeValue,
  type SchedulingThresholdWindowRules as WindowRules
} from '@/utils/schedulingThresholdRange'

defineProps<{
  // Fixed per-platform thresholds from the main settings form, for the hint.
  thresholds?: Record<string, number>
}>()

interface RangeFormEntry {
  enabled: boolean
  value: SchedulingThresholdRangeValue
}

const { t } = useI18n()
const ranges = reactive<Record<string, RangeFormEntry>>({})
const windowRules = reactive<Record<string, WindowRules>>({})
const loaded = ref(false)
const loading = ref(false)
const error = ref('')
const saved = ref(false)

function applyRanges({ ranges: stored, window_rules: storedRules }: AccountSchedulingThresholdCustomSettings) {
  for (const platform of SCHEDULING_THRESHOLD_PLATFORMS) {
    windowRules[platform] = parseSchedulingThresholdWindowRules(storedRules[platform], platform)
    const entry = stored[platform]
    ranges[platform] = {
      enabled: entry?.enabled === true,
      value: entry && entry.max > 0
        ? { min: entry.min, max: entry.max, full_monthly_limit: entry.full_monthly_limit ?? 0 }
        : { ...DEFAULT_SCHEDULING_THRESHOLD_RANGE }
    }
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyRanges(await accountSchedulingThresholdRangesAPI.get())
    loaded.value = true
  } catch {
    error.value = t('admin.settings.schedulingThresholdRange.loadError')
  } finally {
    loading.value = false
  }
}

async function save() {
  error.value = ''
  saved.value = false
  const payload: AccountSchedulingThresholdRanges = {}
  const rulesPayload: AccountSchedulingThresholdWindowRuleMap = {}
  for (const platform of SCHEDULING_THRESHOLD_PLATFORMS) {
    const entry = ranges[platform]
    if (entry.enabled && !isValidSchedulingThresholdRange(entry.value)) {
      error.value = t('admin.settings.schedulingThresholdRange.invalid', { platform })
      return
    }
    const rules = windowRules[platform] ?? {}
    if (!isValidSchedulingThresholdWindowRules(rules)) {
      error.value = t('admin.settings.schedulingThresholdRange.windowRules.invalid')
      return
    }
    if (Object.keys(rules).length > 0) rulesPayload[platform] = rules
    // Disabled entries keep their numbers so turning them back on restores them.
    if (entry.enabled || isValidSchedulingThresholdRange(entry.value)) {
      payload[platform] = { enabled: entry.enabled, ...entry.value }
    }
  }
  loading.value = true
  try {
    applyRanges(await accountSchedulingThresholdRangesAPI.save({ ranges: payload, window_rules: rulesPayload }))
    saved.value = true
  } catch {
    error.value = t('admin.settings.schedulingThresholdRange.error')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
