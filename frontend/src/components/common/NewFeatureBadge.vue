<template>
  <span v-if="false" />
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{
  noticeId: string
  userId: number | null | undefined
  compact?: boolean
}>(), {
  compact: false,
})
</script>
