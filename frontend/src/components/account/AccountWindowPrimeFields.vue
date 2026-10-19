<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="account-window-prime-section">
    <label class="input-label mb-0">{{ t('admin.settings.windowPrime.account.title') }}</label>
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.windowPrime.account.hint') }}</p>
    <div class="mt-3 flex flex-wrap gap-4" role="radiogroup" :aria-label="t('admin.settings.windowPrime.account.mode')">
      <label v-for="mode in modes" :key="mode.value" class="flex items-center gap-2 text-sm">
        <input
          type="radio"
          :value="mode.value"
          :checked="modelValue.mode === mode.value"
          :data-testid="`account-window-prime-mode-${mode.value}`"
          @change="emitConfig({ mode: mode.value })"
        />
        {{ t(mode.label) }}
      </label>
    </div>
    <template v-if="modelValue.mode !== 'off'">
      <label class="input-label mt-3">{{ t('admin.settings.windowPrime.model') }}</label>
      <WindowPrimeModelSelect
        :model-value="modelValue.model"
        :options="upstreamModels"
        :empty-label="t('admin.settings.windowPrime.modelFollow')"
        :load-failed="modelsLoadFailed"
        test-id="account-window-prime-model"
        @update:model-value="emitConfig({ model: $event })"
      />
      <label class="input-label mt-3">{{ t('admin.settings.windowPrime.prompts') }}</label>
      <textarea
        v-model="promptsText"
        rows="3"
        class="input"
        :placeholder="t('admin.settings.windowPrime.account.promptsPlaceholder')"
        data-testid="account-window-prime-prompts"
        @input="emitConfig({ prompts: splitWindowPrimePrompts(promptsText) })"
      />
      <p class="input-hint">{{ t('admin.settings.windowPrime.promptsHint') }}</p>
      <p v-if="!isValidWindowPrimePrompts(modelValue.prompts)" role="alert" class="text-xs text-red-500">
        {{ t('admin.settings.windowPrime.invalid') }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import WindowPrimeModelSelect from '@/components/settings/WindowPrimeModelSelect.vue'
import { getAvailableModels } from '@/api/admin/accounts'
import {
  isValidWindowPrimePrompts,
  splitWindowPrimePrompts,
  type AccountWindowPrimeConfig,
  type WindowPrimeMode
} from '@/utils/accountWindowPrime'

const props = defineProps<{
  modelValue: AccountWindowPrimeConfig
  // Loads this account's upstream models for the picker.
  accountId?: number
}>()
const emit = defineEmits<{ (e: 'update:modelValue', value: AccountWindowPrimeConfig): void }>()

const { t } = useI18n()
const modes: Array<{ value: WindowPrimeMode; label: string }> = [
  { value: 'inherit', label: 'admin.settings.windowPrime.account.modeInherit' },
  { value: 'on', label: 'admin.settings.windowPrime.account.modeOn' },
  { value: 'off', label: 'admin.settings.windowPrime.account.modeOff' }
]
const promptsText = ref(props.modelValue.prompts.join('\n'))

// Reload the text when another account is opened, not on every keystroke.
watch(
  () => props.modelValue.prompts,
  (prompts) => {
    if (splitWindowPrimePrompts(promptsText.value).join('\n') !== prompts.join('\n')) {
      promptsText.value = prompts.join('\n')
    }
  }
)

const upstreamModels = ref<string[]>([])
const modelsLoadFailed = ref(false)

async function loadUpstreamModels(accountId: number | undefined) {
  upstreamModels.value = []
  modelsLoadFailed.value = false
  if (!accountId) return
  try {
    upstreamModels.value = (await getAvailableModels(accountId)).map((model) => model.id)
  } catch {
    modelsLoadFailed.value = true
  }
}

onMounted(() => loadUpstreamModels(props.accountId))
watch(() => props.accountId, loadUpstreamModels)

function emitConfig(patch: Partial<AccountWindowPrimeConfig>) {
  emit('update:modelValue', { ...props.modelValue, ...patch })
}
</script>
