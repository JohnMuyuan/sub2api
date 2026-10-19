<template>
  <div class="space-y-3">
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="input-label">{{ t('admin.settings.schedulingThresholdRange.min') }}</label>
        <input
          :value="modelValue.min"
          type="number"
          min="1"
          max="100"
          step="1"
          class="input"
          :data-testid="`${testIdPrefix}-min`"
          @input="update('min', $event)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.settings.schedulingThresholdRange.max') }}</label>
        <input
          :value="modelValue.max"
          type="number"
          min="1"
          max="100"
          step="1"
          class="input"
          :data-testid="`${testIdPrefix}-max`"
          @input="update('max', $event)"
        />
      </div>
    </div>
    <p class="input-hint">{{ t('admin.settings.schedulingThresholdRange.rangeHint') }}</p>
    <div>
      <label class="input-label">{{ t('admin.settings.schedulingThresholdRange.fullMonthlyLimit') }}</label>
      <input
        :value="modelValue.full_monthly_limit"
        type="number"
        min="0"
        :max="MAX_FULL_MONTHLY_LIMIT"
        step="1"
        class="input"
        :placeholder="t('admin.settings.schedulingThresholdRange.fullMonthlyLimitPlaceholder')"
        :data-testid="`${testIdPrefix}-full-monthly-limit`"
        @input="update('full_monthly_limit', $event)"
      />
      <p class="mt-1 text-xs text-amber-600 dark:text-amber-400">
        {{ t('admin.settings.schedulingThresholdRange.fullHint') }}
      </p>
      <p class="input-hint">{{ t('admin.settings.schedulingThresholdRange.fullLimitHint') }}</p>
    </div>
    <p v-if="!valid" role="alert" class="text-xs text-red-500" :data-testid="`${testIdPrefix}-invalid`">
      {{ t('admin.settings.schedulingThresholdRange.invalidAccount') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  MAX_FULL_MONTHLY_LIMIT,
  isValidSchedulingThresholdRange,
  type SchedulingThresholdRangeValue
} from '@/utils/schedulingThresholdRange'

const props = defineProps<{
  modelValue: SchedulingThresholdRangeValue
  testIdPrefix: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: SchedulingThresholdRangeValue): void
}>()

const { t } = useI18n()
const valid = computed(() => isValidSchedulingThresholdRange(props.modelValue))

function update(field: keyof SchedulingThresholdRangeValue, event: Event) {
  const raw = (event.target as HTMLInputElement).value
  emit('update:modelValue', { ...props.modelValue, [field]: raw === '' ? Number.NaN : Number(raw) })
}
</script>
