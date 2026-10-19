<template>
  <div
    v-if="state"
    class="mt-1 text-[11px]"
    :class="state.succeeded ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'"
    :title="state.error"
    data-testid="account-window-prime-status"
  >
    {{
      state.succeeded
        ? t('admin.settings.windowPrime.status.succeeded', { windows: state.windows, time: state.time })
        : t('admin.settings.windowPrime.status.failed', { windows: state.windows, attempts: state.attempts })
    }}
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTimeToMinute } from '@/utils/format'

// Account extra is a typed intersection; only window_prime_state is read here.
const props = defineProps<{ extra?: object }>()
const { t } = useI18n()

// Older results stop being useful once the next window is under way.
const STATUS_MAX_AGE_MS = 24 * 60 * 60 * 1000

const state = computed(() => {
  const raw = (props.extra as Record<string, unknown> | undefined)?.window_prime_state
  if (!raw || typeof raw !== 'object') return null
  const source = raw as Record<string, unknown>
  const at = typeof source.last_attempt_at === 'string' ? new Date(source.last_attempt_at) : null
  if (!at || Number.isNaN(at.getTime()) || Date.now() - at.getTime() > STATUS_MAX_AGE_MS) return null
  const windows = Array.isArray(source.windows) ? source.windows.filter((w) => typeof w === 'string').join('·') : ''
  return {
    succeeded: source.status === 'succeeded',
    windows,
    time: formatDateTimeToMinute(at),
    attempts: Number(source.attempts) || 1,
    error: typeof source.error === 'string' ? source.error : undefined
  }
})
</script>
