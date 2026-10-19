<template>
  <div
    v-if="rounds.length"
    class="mb-1 flex flex-wrap items-center gap-1 text-[11px]"
    data-testid="scheduling-threshold-rounds"
  >
    <span class="text-gray-500 dark:text-gray-400">{{ t('admin.settings.schedulingThresholdRange.rounds.title') }}</span>
    <span
      v-for="round in rounds"
      :key="`${round.window}|${round.scope ?? ''}`"
      class="rounded px-1.5 py-0.5 font-medium"
      :class="round.threshold_percent >= 100
        ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
        : 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'"
      :title="tooltip(round)"
      data-testid="scheduling-threshold-round"
    >
      {{ label(round) }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { formatDateTimeToMinute } from '@/utils/format'

type Round = NonNullable<Account['scheduling_threshold_rounds']>[number]

const props = defineProps<{
  rounds?: Account['scheduling_threshold_rounds']
}>()

const { t, te } = useI18n()
const rounds = computed(() => props.rounds ?? [])

function windowName(round: Round) {
  const key = `admin.settings.schedulingThresholdRange.windows.${round.window}`
  return te(key) ? t(key) : round.window
}

function label(round: Round) {
  const window = windowName(round)
  return round.threshold_percent >= 100
    ? t('admin.settings.schedulingThresholdRange.rounds.noStop', { window })
    : t('admin.settings.schedulingThresholdRange.rounds.stopAt', { window, percent: round.threshold_percent })
}

function tooltip(round: Round) {
  const lines = [
    t('admin.settings.schedulingThresholdRange.rounds.tooltip', {
      min: round.range_min,
      max: round.range_max,
      resetsAt: formatDateTimeToMinute(round.resets_at)
    })
  ]
  if (round.full_monthly_limit > 0) {
    lines.push(t('admin.settings.schedulingThresholdRange.rounds.fullDraws', {
      used: round.full_draws_used,
      limit: round.full_monthly_limit
    }))
  }
  return lines.join('\n')
}
</script>
