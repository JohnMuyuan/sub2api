<template>
  <div class="space-y-2">
    <select class="input" :value="selectValue" :data-testid="testId" @change="onSelect(($event.target as HTMLSelectElement).value)">
      <option value="">{{ emptyLabel }}</option>
      <option v-for="model in options" :key="model" :value="model">{{ model }}</option>
      <option :value="CUSTOM">{{ t('admin.settings.windowPrime.modelCustom') }}</option>
    </select>
    <input
      v-if="custom"
      :value="modelValue"
      type="text"
      class="input"
      :placeholder="t('admin.settings.windowPrime.modelCustomPlaceholder')"
      :data-testid="`${testId}-custom`"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <p v-if="loadFailed" class="text-xs text-amber-600 dark:text-amber-400">
      {{ t('admin.settings.windowPrime.modelLoadFailed') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const CUSTOM = '__custom__'

const props = defineProps<{
  modelValue: string
  options: string[]
  // Label for the empty value, e.g. the default model or "follow platform".
  emptyLabel: string
  testId: string
  loadFailed?: boolean
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const { t } = useI18n()
const isListed = (value: string) => value === '' || props.options.includes(value)
const custom = ref(!isListed(props.modelValue))

// A saved model that appears once the list loads is no longer "custom"; an
// emptied custom box stays open while the user is typing.
watch(
  () => [props.modelValue, props.options] as const,
  ([value]) => {
    if (!isListed(value)) custom.value = true
    else if (value !== '') custom.value = false
  }
)

const selectValue = computed(() => (custom.value ? CUSTOM : props.modelValue))

function onSelect(value: string) {
  if (value === CUSTOM) {
    custom.value = true
    return
  }
  custom.value = false
  emit('update:modelValue', value)
}
</script>
