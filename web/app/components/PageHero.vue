<script setup lang="ts">
import Title from 'fuxsto-design/title'

defineProps<{
  eyebrow: string
  title: string
  subtitle: string
  gradient?: boolean
}>()
</script>

<template>
  <header class="page-hero">
    <p class="eyebrow">{{ eyebrow }}</p>
    <Title :level="1" :gradient="gradient" class="tracking-tight">{{ title }}</Title>
    <p class="mx-auto max-w-xl text-lg text-muted-foreground sm:text-xl">
      {{ subtitle }}
    </p>
    <slot />
  </header>
</template>
