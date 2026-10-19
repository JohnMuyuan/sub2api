<template>
  <div v-if="windows.length" class="space-y-3" :data-testid="testIdPrefix">
    <div>
      <label class="input-label mb-0">{{ t('admin.settings.schedulingThresholdRange.windowRules.title') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ hint }}</p>
    </div>
    <div
      v-for="window in windows"
      :key="window"
      class="rounded-md border border-gray-200 p-3 dark:border-dark-600"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ windowLabel(window) }}</span>
        <select
          class="input w-auto"
          :value="modelValue[window]?.mode ?? 'inherit'"
          :data-testid="`${testIdPrefix}-${window}-mode`"
          @change="setMode(window, ($event.target as HTMLSelectElement).value)"
        >
          <option value="inherit">{{ t('admin.settings.schedulingThresholdRange.windowRules.inherit') }}</option>
          <option value="fixed">{{ t('admin.settings.schedulingThresholdRange.windowRules.fixed') }}</option>
          <option value="random">{{ t('admin.settings.schedulingThresholdRange.windowRules.random') }}</option>
        </select>
      </div>
      <p v-if="window === '7d_oi'" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
        {{ t('admin.settings.schedulingThresholdRange.windowRules.fableHint') }}
      </p>
      <div v-if="modelValue[window]?.mode === 'fixed'" class="mt-2">
        <label class="input-label">{{ t('admin.settings.schedulingThresholdRange.windowRules.threshold') }}</label>
        <input
          :value="modelValue[window]?.threshold"
          type="number"
          min="1"
          max="100"
          step="1"
          class="input"
          :data-testid="`${testIdPrefix}-${window}-threshold`"
          @input="setThreshold(window, ($event.target as HTMLInputElement).value)"
        />
        <p class="input-hint">{{ t('admin.settings.schedulingThresholdRange.windowRules.thresholdHint') }}</p>
      </div>
      <SchedulingThresholdRangeFields
        v-else-if="modelValue[window]?.mode === 'random'"
        class="mt-2"
        :model-value="rangeOf(window)"
        :test-id-prefix="`${testIdPrefix}-${window}`"
        @update:model-value="setRange(window, $event)"
      />
    </div>
    <p v-if="!isValidSchedulingThresholdWindowRules(modelValue)" role="alert" class="text-xs text-red-500">
      {{ t('admin.settings.schedulingThresholdRange.windowRules.invalid') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SchedulingThresholdRangeFields from './SchedulingThresholdRangeFields.vue'
import {
  DEFAULT_SCHEDULING_THRESHOLD_RANGE,
  SCHEDULING_THRESHOLD_WINDOWS,
  isValidSchedulingThresholdWindowRules,
  type SchedulingThresholdRangeValue,
  type SchedulingThresholdWindowRules
} from '@/utils/schedulingThresholdRange'

const props = defineProps<{
  platform: string | undefined
  modelValue: SchedulingThresholdWindowRules
  testIdPrefix: string
  // Account forms explain that platform rules stop applying.
  account?: boolean
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: SchedulingThresholdWindowRules): void }>()

const { t } = useI18n()
const windows = computed(() => SCHEDULING_THRESHOLD_WINDOWS[props.platform ?? ''] ?? [])
const hint = computed(() =>
  t(props.account
    ? 'admin.settings.schedulingThresholdRange.windowRules.accountHint'
    : 'admin.settings.schedulingThresholdRange.windowRules.hint')
)

function windowLabel(window: string) {
  const key = `admin.settings.schedulingThresholdRange.windows.${window}`
  const label = t(key)
  return label === key ? window : label
}

function update(window: string, rule: SchedulingThresholdWindowRules[string] | undefined) {
  const next = { ...props.modelValue }
  if (rule) next[window] = rule
  else delete next[window]
  emit('update:modelValue', next)
}

function setMode(window: string, mode: string) {
  if (mode === 'fixed') update(window, { mode: 'fixed', threshold: 95 })
  else if (mode === 'random') update(window, { mode: 'random', ...DEFAULT_SCHEDULING_THRESHOLD_RANGE })
  else update(window, undefined)
}

function setThreshold(window: string, raw: string) {
  update(window, { mode: 'fixed', threshold: raw === '' ? Number.NaN : Number(raw) })
}

function rangeOf(window: string): SchedulingThresholdRangeValue {
  const rule = props.modelValue[window]
  return {
    min: rule?.min ?? DEFAULT_SCHEDULING_THRESHOLD_RANGE.min,
    max: rule?.max ?? DEFAULT_SCHEDULING_THRESHOLD_RANGE.max,
    full_monthly_limit: rule?.full_monthly_limit ?? 0
  }
}

function setRange(window: string, value: SchedulingThresholdRangeValue) {
  update(window, { mode: 'random', ...value })
}
</script>
