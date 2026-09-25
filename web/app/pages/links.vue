<script setup lang="ts">
import { ExternalLink, Link2 } from 'lucide-vue-next'
import Alert from 'fuxsto-design/alert'
import Card from 'fuxsto-design/card'
import Empty from 'fuxsto-design/empty'
import Skeleton from 'fuxsto-design/skeleton'
import { fetchFriends, ApiError, type FriendLink } from '~/utils/api'
import { errorMessage } from '~/utils/format'

useHead({ title: '友情链接 · li-gh-proxy' })

const friends = ref<FriendLink[]>([])
const loading = ref(true)
const disabled = ref(false)
const error = ref('')
// 头像加载失败的条目降级为文字徽标
const brokenAvatars = ref(new Set<string>())

const hasFriends = computed(() => friends.value.length > 0)

function initialOf(link: FriendLink) {
  return link.name.trim().charAt(0).toUpperCase() || '?'
}

onMounted(async () => {
  try {
    const res = await fetchFriends()
    friends.value = res.items || []
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      disabled.value = true
    } else {
      error.value = errorMessage(e, '加载友情链接失败')
    }
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <PageHero
      eyebrow="LINKS"
      title="友情链接"
      subtitle="访问本站的朋友们，排名不分先后。"
    />

    <div class="mx-auto max-w-3xl">
      <div v-if="loading" class="pt-2">
        <Skeleton :rows="3" :height="88" round />
      </div>

      <Empty
        v-else-if="disabled"
        title="功能未开启"
        description="站长可在服务端配置文件的 [friends] 段启用友情链接功能"
        class="pt-6"
      />

      <template v-else-if="error">
        <Alert type="error" :title="error" />
      </template>

      <Empty
        v-else-if="!hasFriends"
        title="暂无友链"
        description="站长可在服务端数据目录中添加友链文件"
        class="pt-6"
      />

      <div v-else class="grid gap-4 sm:grid-cols-2">
        <Card
          v-for="link in friends"
          :key="link.slug"
          padding="none"
          class="group overflow-hidden transition-colors duration-150 hover:border-primary/50"
        >
          <a
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
            class="flex items-start gap-3 p-4"
          >
            <span
              v-if="link.avatar && !brokenAvatars.has(link.slug)"
              class="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border bg-muted/40"
            >
              <img
                :src="link.avatar"
                :alt="link.name"
                class="size-full object-cover"
                loading="lazy"
                @error="brokenAvatars.add(link.slug)"
              />
            </span>
            <span
              v-else
              class="flex size-11 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/40 text-base font-semibold text-muted-foreground"
            >
              {{ initialOf(link) }}
            </span>

            <span class="min-w-0 flex-1 space-y-1">
              <span class="flex items-center gap-1.5 font-medium">
                <span class="truncate">{{ link.name }}</span>
                <ExternalLink
                  class="size-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity duration-150 group-hover:opacity-100"
                />
              </span>
              <span class="line-clamp-2 block text-sm text-muted-foreground">
                {{ link.description || link.url }}
              </span>
            </span>
          </a>
        </Card>
      </div>

      <p
        v-if="!loading && !disabled && !error && hasFriends"
        class="flex items-center justify-center gap-1.5 pt-8 text-sm text-muted-foreground"
      >
        <Link2 class="size-3.5" />
        由站长在服务器本地以文件方式维护
      </p>
    </div>
  </div>
</template>
