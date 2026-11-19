<template>
  <div class="space-y-3">
    <label class="block text-xs">{{ t('admin.accounts.pelicanTest.question') }}</label>
    <Select :model-value="selectedOptionValue" :options="questionOptions" @update:model-value="selectQuestion" />
    <p v-if="isProbe" class="text-xs text-gray-500 dark:text-gray-400" data-testid="state-probe-hint">{{ t('admin.accounts.pelicanTest.stateProbeHint') }}</p>
    <template v-else>
      <PelicanPromptMode v-if="!modelValue.question_kind || modelValue.question_kind === 'pelican'" :model-value="modelValue.prompt_mode" @update:model-value="update('prompt_mode', $event)" />
      <TextArea v-if="modelValue.prompt_mode !== 'random'" :model-value="modelValue.prompt" :label="t('admin.accounts.pelicanTest.promptLabel')" :rows="3" @update:model-value="update('prompt', $event)" />
      <Input
        v-if="isCustomKind"
        :model-value="modelValue.delivery_contract || ''"
        :label="t('admin.accounts.pelicanTest.customQuestions.contractLabel')"
        :placeholder="t('admin.accounts.pelicanTest.customQuestions.contractPlaceholder')"
        :hint="t('admin.accounts.pelicanTest.customQuestions.contractHint')"
        @update:model-value="update('delivery_contract', String($event ?? ''))"
      />
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.accounts.pelicanTest.reasoning') }}</label>
          <Select :model-value="modelValue.reasoning_effort" :options="reasoningOptions" @update:model-value="update('reasoning_effort', $event)" />
        </div>
        <Input :model-value="String(modelValue.parallel_count)" type="number" :label="t('admin.accounts.pelicanTest.parallel')" :hint="t('admin.accounts.pelicanTest.parallelHint')" @update:model-value="update('parallel_count', Number($event))" />
      </div>
    </template>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import PelicanPromptMode from './PelicanPromptMode.vue'
import { customQuestionKind, isCustomQuestionKind, questionPrompt, STATE_PROBE_QUESTION } from '@/utils/intelligenceTest'
import { useI18n } from 'vue-i18n'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import type { IQCustomQuestion, PelicanTestConfig } from '@/types'
const props = defineProps<{ modelValue: PelicanTestConfig; customQuestions?: IQCustomQuestion[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: PelicanTestConfig] }>()
const { t } = useI18n()
const isProbe = computed(() => props.modelValue.question_kind === STATE_PROBE_QUESTION)
const isCustomKind = computed(() => isCustomQuestionKind(props.modelValue.question_kind))
const reasoningOptions = computed(() => ['low', 'medium', 'high'].map(value => ({ value, label: t(`admin.accounts.pelicanTest.reasoning${value[0].toUpperCase()}${value.slice(1)}`) })))
const selectedOptionValue = computed(() => {
  const kind = props.modelValue.question_kind || 'pelican'
  if (isCustomQuestionKind(kind) && props.customQuestions?.length) {
    const matched = props.customQuestions.find((item) =>
      customQuestionKind(item.output_type) === kind &&
      item.prompt === props.modelValue.prompt &&
      (item.delivery_contract || '') === (props.modelValue.delivery_contract || '')
    )
    if (matched) return `saved:${matched.id}`
  }
  return kind
})
const questionOptions = computed(() => [
  ...['candy', 'pelican'].map(value => ({ value, label: t(`admin.accounts.pelicanTest.${value}Question`) })),
  { value: 'custom', label: t('admin.accounts.pelicanTest.customTextQuestion') },
  { value: 'custom_html', label: t('admin.accounts.pelicanTest.customHtmlQuestion') },
  ...(props.customQuestions || []).map((item) => ({
    value: `saved:${item.id}`,
    label: item.output_type === 'html'
      ? `${item.title} (${t('admin.accounts.pelicanTest.customQuestionTag')} · ${t('admin.accounts.pelicanTest.customHtmlTag')})`
      : `${item.title} (${t('admin.accounts.pelicanTest.customQuestionTag')})`
  })),
  { value: STATE_PROBE_QUESTION, label: t('admin.accounts.pelicanTest.stateProbeQuestion') }
])
function selectQuestion(value: string | number | boolean | null) {
  if (value === STATE_PROBE_QUESTION) {
    // 探针不需要题目；同一账号同一时刻只能跑一次探针，并行固定为 1。
    emit('update:modelValue', { ...props.modelValue, prompt_mode: 'custom', question_kind: STATE_PROBE_QUESTION, prompt: '', delivery_contract: undefined, parallel_count: 1, reasoning_effort: props.modelValue.reasoning_effort || 'medium' })
    return
  }
  if (value === 'candy' || value === 'pelican') {
    emit('update:modelValue', { ...props.modelValue, prompt_mode: 'custom', question_kind: value, prompt: questionPrompt(value), delivery_contract: undefined })
    return
  }
  if (value === 'custom' || value === 'custom_html') {
    const keepPrompt = isCustomQuestionKind(props.modelValue.question_kind)
    emit('update:modelValue', { ...props.modelValue, prompt_mode: 'custom', question_kind: value, prompt: keepPrompt ? props.modelValue.prompt : '', delivery_contract: keepPrompt ? props.modelValue.delivery_contract : '' })
    return
  }
  if (typeof value === 'string' && value.startsWith('saved:')) {
    const id = value.slice('saved:'.length)
    const found = (props.customQuestions || []).find((item) => item.id === id)
    if (!found) return
    emit('update:modelValue', {
      ...props.modelValue,
      prompt_mode: 'custom',
      question_kind: customQuestionKind(found.output_type),
      prompt: found.prompt,
      delivery_contract: found.delivery_contract || ''
    })
  }
}
function update(key: keyof PelicanTestConfig, value: string | number | boolean | null) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>
