<script setup lang="ts">
import { Globe } from 'lucide-vue-next'
import Chip from 'fuxsto-design/chip'

const { nodes, selectedUrl, select } = useNodes()
</script>

<template>
  <div v-if="nodes.length > 0" class="space-y-3">
    <p class="text-center text-sm text-muted-foreground">选择加速节点（影响下方生成的链接）</p>
    <div class="flex flex-wrap justify-center gap-2">
      <Chip
        round
        selectable
        :selected="!selectedUrl"
        :icon="Globe"
        @click="select('')"
      >
        当前站点
      </Chip>
      <Chip
        v-for="node in nodes"
        :key="node.url"
        round
        selectable
        :selected="selectedUrl === node.url"
        @click="select(node.url)"
      >
        {{ node.name }}
      </Chip>
    </div>
  </div>
</template>
