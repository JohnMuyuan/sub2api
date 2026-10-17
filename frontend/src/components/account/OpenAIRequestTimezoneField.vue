<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="input-label">{{ t('admin.accounts.openai.requestTimezone') }}</label>
    <Select
      :model-value="modelValue"
      :options="options"
      :loading="loading"
      searchable
      :aria-label="t('admin.accounts.openai.requestTimezone')"
      @update:model-value="selectTimezone"
    />
    <p class="input-hint">{{ t(generic ? 'admin.accounts.openai.genericRequestTimezoneDesc' : 'admin.accounts.openai.requestTimezoneDesc') }}</p>
    <p v-if="loadFailed" class="mt-1 text-xs text-red-500">{{ t('admin.accounts.openai.requestTimezoneLoadFailed') }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'

const props = defineProps<{ modelValue: string; generic?: boolean }>()
const emit = defineEmits<{ (event: 'update:modelValue', value: string): void }>()
const { t } = useI18n()
const specialOptions = () => props.generic ? [
  { value: '', label: t('admin.accounts.openai.requestTimezonePreserve') },
  { value: 'auto', label: t('admin.accounts.openai.requestTimezoneAuto') }
] : []
const fallback = () => props.generic ? '' : 'Asia/Singapore'
const options = ref([...specialOptions(), { value: 'Asia/Singapore', label: 'Asia/Singapore' }])
const loading = ref(false)
const loadFailed = ref(false)
const loaded = ref(false)

const selectTimezone = (value: string | number | boolean | null) => {
  if (typeof value === 'string') emit('update:modelValue', value)
}

watch(() => props.modelValue, value => {
  if (loaded.value && !options.value.some(option => option.value === value)) emit('update:modelValue', fallback())
})

onMounted(async () => {
  loading.value = true
  try {
    const result = await adminAPI.accounts.getOpenAIRequestTimezones()
    options.value = [...specialOptions(), ...result.timezones.map(value => ({ value, label: value }))]
    loaded.value = true
    if (!options.value.some(option => option.value === props.modelValue)) emit('update:modelValue', props.generic ? '' : result.default)
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
})
</script>
