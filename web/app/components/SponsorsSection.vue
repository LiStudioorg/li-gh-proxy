<script setup lang="ts">
import { HeartHandshake } from 'lucide-vue-next'
import Card from 'fuxsto-design/card'
import { fetchSponsors, type Sponsor } from '~/utils/api'

// 接口 404（未启用）或无数据时整块不渲染
const sponsors = ref<Sponsor[]>([])
const brokenLogos = ref(new Set<string>())

onMounted(async () => {
  try {
    const res = await fetchSponsors()
    sponsors.value = res.items || []
  } catch {
    sponsors.value = []
  }
})
</script>

<template>
  <section v-if="sponsors.length > 0" class="section-gap space-y-6">
    <div class="space-y-1 text-center">
      <h2 class="text-sm font-semibold tracking-[0.16em] text-muted-foreground uppercase">
        Sponsors
      </h2>
      <p class="text-muted-foreground">感谢赞助商对本项目的支持</p>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <Card
        v-for="item in sponsors"
        :key="item.slug"
        padding="none"
        class="group overflow-hidden transition-colors duration-150 hover:border-primary/50"
      >
        <a
          :href="item.url"
          target="_blank"
          rel="noopener noreferrer"
          class="flex items-center gap-3 p-4"
        >
          <span
            v-if="item.logo && !brokenLogos.has(item.slug)"
            class="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border bg-muted/40"
          >
            <img
              :src="item.logo"
              :alt="item.name"
              class="size-full object-contain"
              loading="lazy"
              @error="brokenLogos.add(item.slug)"
            />
          </span>
          <span
            v-else
            class="flex size-11 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/40"
          >
            <HeartHandshake class="size-5 text-muted-foreground" />
          </span>

          <span class="min-w-0 flex-1 space-y-0.5">
            <span class="block truncate font-medium">{{ item.name }}</span>
            <span
              v-if="item.description"
              class="line-clamp-2 block text-sm text-muted-foreground"
            >
              {{ item.description }}
            </span>
          </span>
        </a>
      </Card>
    </div>
  </section>
</template>
