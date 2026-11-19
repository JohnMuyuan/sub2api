<template>
  <form
    class="space-y-4 rounded-xl border border-primary-200 bg-primary-50/40 p-4 dark:border-primary-800/70 dark:bg-primary-950/20"
    :aria-labelledby="headingId"
    novalidate
    data-testid="custom-question-editor"
    @submit.prevent="handleSubmit"
  >
    <div class="flex items-center justify-between gap-2">
      <h3 :id="headingId" class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ editing ? t('admin.accounts.pelicanTest.customQuestions.editTitle') : t('admin.accounts.pelicanTest.customQuestions.createTitle') }}
      </h3>
      <button
        type="button"
        class="rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200"
        :disabled="saving"
        @click="$emit('cancel')"
      >
        {{ t('common.cancel') }}
      </button>
    </div>

    <p
      v-if="serverError"
      role="alert"
      class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-800/60 dark:bg-red-950/30 dark:text-red-300"
      data-testid="custom-question-error"
    >
      {{ serverError }}
    </p>

    <div class="space-y-1.5">
      <label :for="`${uid}-title`" class="block text-xs font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.pelicanTest.customQuestions.titleLabel') }}
        <span aria-hidden="true" class="text-red-500">*</span>
      </label>
      <input
        :id="`${uid}-title`"
        ref="titleInput"
        v-model="form.title"
        name="title"
        type="text"
        required
        maxlength="100"
        class="input w-full"
        :class="titleError ? 'border-red-500 focus:border-red-500 focus:ring-red-500' : ''"
        :placeholder="t('admin.accounts.pelicanTest.customQuestions.titlePlaceholder')"
        :aria-invalid="titleError ? 'true' : 'false'"
        :aria-describedby="titleError ? `${uid}-title-error` : undefined"
        :disabled="saving"
        data-testid="custom-question-title"
        @input="onTitleInput"
        @blur="onTitleBlur"
      />
      <p v-if="titleError" :id="`${uid}-title-error`" class="text-xs text-red-600 dark:text-red-400" data-testid="custom-question-title-error">
        {{ titleError }}
      </p>
    </div>

    <fieldset class="space-y-2" :disabled="saving">
      <legend class="text-xs font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.pelicanTest.customQuestions.outputTypeLabel') }}
      </legend>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <label
          :for="`${uid}-output-text`"
          class="flex cursor-pointer items-start gap-2.5 rounded-lg border p-2.5 text-xs transition-colors"
          :class="form.output_type === 'text' ? 'border-primary-500 bg-white dark:border-primary-400 dark:bg-dark-800' : 'border-gray-200 bg-white/70 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800/50'"
        >
          <input
            :id="`${uid}-output-text`"
            v-model="form.output_type"
            type="radio"
            name="output_type"
            value="text"
            class="mt-0.5 accent-primary-600"
            data-testid="custom-question-output-text"
          />
          <span>
            <span class="block font-medium text-gray-900 dark:text-gray-100">{{ t('admin.accounts.pelicanTest.customQuestions.outputText') }}</span>
            <span class="mt-0.5 block text-gray-500 dark:text-gray-400">{{ t('admin.accounts.pelicanTest.customQuestions.outputTextDesc') }}</span>
          </span>
        </label>
        <label
          :for="`${uid}-output-html`"
          class="flex cursor-pointer items-start gap-2.5 rounded-lg border p-2.5 text-xs transition-colors"
          :class="form.output_type === 'html' ? 'border-primary-500 bg-white dark:border-primary-400 dark:bg-dark-800' : 'border-gray-200 bg-white/70 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800/50'"
        >
          <input
            :id="`${uid}-output-html`"
            v-model="form.output_type"
            type="radio"
            name="output_type"
            value="html"
            class="mt-0.5 accent-primary-600"
            data-testid="custom-question-output-html"
          />
          <span>
            <span class="block font-medium text-gray-900 dark:text-gray-100">{{ t('admin.accounts.pelicanTest.customQuestions.outputHtml') }}</span>
            <span class="mt-0.5 block text-gray-500 dark:text-gray-400">{{ t('admin.accounts.pelicanTest.customQuestions.outputHtmlDesc') }}</span>
          </span>
        </label>
      </div>
    </fieldset>

    <div class="space-y-1.5">
      <label :for="`${uid}-prompt`" class="block text-xs font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.pelicanTest.customQuestions.promptLabel') }}
        <span aria-hidden="true" class="text-red-500">*</span>
      </label>
      <textarea
        :id="`${uid}-prompt`"
        ref="promptInput"
        v-model="form.prompt"
        name="prompt"
        rows="4"
        required
        class="input w-full resize-y"
        :class="promptError ? 'border-red-500 focus:border-red-500 focus:ring-red-500' : ''"
        :placeholder="t('admin.accounts.pelicanTest.customQuestions.promptPlaceholder')"
        :aria-invalid="promptError ? 'true' : 'false'"
        :aria-describedby="promptError ? `${uid}-prompt-error` : undefined"
        :disabled="saving"
        data-testid="custom-question-prompt"
        @input="onPromptInput"
        @blur="onPromptBlur"
      />
      <p v-if="promptError" :id="`${uid}-prompt-error`" class="text-xs text-red-600 dark:text-red-400" data-testid="custom-question-prompt-error">
        {{ promptError }}
      </p>
    </div>

    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div class="space-y-1.5">
        <label :for="`${uid}-contract`" class="block text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.pelicanTest.customQuestions.contractLabel') }}
        </label>
        <input
          :id="`${uid}-contract`"
          v-model="form.delivery_contract"
          name="delivery_contract"
          type="text"
          class="input w-full"
          :placeholder="t('admin.accounts.pelicanTest.customQuestions.contractPlaceholder')"
          :aria-describedby="`${uid}-contract-hint`"
          :disabled="saving"
          data-testid="custom-question-contract"
        />
        <p :id="`${uid}-contract-hint`" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.pelicanTest.customQuestions.contractHint') }}
        </p>
      </div>

      <div class="space-y-1.5">
        <label :for="`${uid}-note`" class="block text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.pelicanTest.customQuestions.noteLabel') }}
        </label>
        <input
          :id="`${uid}-note`"
          v-model="form.reference_note"
          name="reference_note"
          type="text"
          class="input w-full"
          :placeholder="t('admin.accounts.pelicanTest.customQuestions.notePlaceholder')"
          :aria-describedby="`${uid}-note-hint`"
          :disabled="saving"
          data-testid="custom-question-note"
        />
        <p :id="`${uid}-note-hint`" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.pelicanTest.customQuestions.noteHint') }}
        </p>
      </div>
    </div>

    <div class="flex justify-end gap-2 pt-1">
      <button type="button" class="btn btn-secondary text-xs" :disabled="saving" @click="$emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" class="btn btn-primary text-xs" :disabled="saving" data-testid="custom-question-submit">
        {{ saving ? t('admin.accounts.pelicanTest.customQuestions.saving') : t('admin.accounts.pelicanTest.customQuestions.save') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { reactive, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { IQCustomQuestionInput } from '@/types'

const props = defineProps<{
  initialValue: IQCustomQuestionInput
  editing?: boolean
  saving?: boolean
  serverError?: string
}>()

const emit = defineEmits<{
  (event: 'save', payload: IQCustomQuestionInput): void
  (event: 'cancel'): void
}>()

const { t } = useI18n()
const uid = useId()
const headingId = `${uid}-heading`

const titleInput = ref<HTMLInputElement | null>(null)
const promptInput = ref<HTMLTextAreaElement | null>(null)

const form = reactive<IQCustomQuestionInput>({
  title: props.initialValue.title || '',
  prompt: props.initialValue.prompt || '',
  output_type: props.initialValue.output_type === 'html' ? 'html' : 'text',
  delivery_contract: props.initialValue.delivery_contract || '',
  reference_note: props.initialValue.reference_note || ''
})

const titleTouched = ref(false)
const promptTouched = ref(false)
const titleError = ref('')
const promptError = ref('')

watch(() => props.initialValue, (val) => {
  form.title = val.title || ''
  form.prompt = val.prompt || ''
  form.output_type = val.output_type === 'html' ? 'html' : 'text'
  form.delivery_contract = val.delivery_contract || ''
  form.reference_note = val.reference_note || ''
  titleTouched.value = false
  promptTouched.value = false
  titleError.value = ''
  promptError.value = ''
}, { deep: true })

function validateTitle(): boolean {
  if (!form.title.trim()) {
    titleError.value = t('admin.accounts.pelicanTest.customQuestions.titleRequired')
    return false
  }
  titleError.value = ''
  return true
}

function validatePrompt(): boolean {
  if (!form.prompt.trim()) {
    promptError.value = t('admin.accounts.pelicanTest.customQuestions.promptRequired')
    return false
  }
  promptError.value = ''
  return true
}

function onTitleInput() {
  titleTouched.value = true
  if (titleError.value && form.title.trim()) titleError.value = ''
}

function onTitleBlur() {
  if (titleTouched.value) validateTitle()
}

function onPromptInput() {
  promptTouched.value = true
  if (promptError.value && form.prompt.trim()) promptError.value = ''
}

function onPromptBlur() {
  if (promptTouched.value) validatePrompt()
}

function handleSubmit() {
  if (props.saving) return
  titleTouched.value = true
  promptTouched.value = true
  const titleOk = validateTitle()
  const promptOk = validatePrompt()
  if (!titleOk) {
    titleInput.value?.focus()
    return
  }
  if (!promptOk) {
    promptInput.value?.focus()
    return
  }
  emit('save', {
    title: form.title.trim(),
    prompt: form.prompt.trim(),
    output_type: form.output_type,
    delivery_contract: (form.delivery_contract || '').trim(),
    reference_note: (form.reference_note || '').trim()
  })
}
</script>
