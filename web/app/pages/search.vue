<script setup lang="ts">
import { ChevronLeft, Copy, Search } from 'lucide-vue-next'
import Button from 'fuxsto-design/button'
import Input from 'fuxsto-design/input'
import Chip from 'fuxsto-design/chip'
import Empty from 'fuxsto-design/empty'
import Skeleton from 'fuxsto-design/skeleton'
import Pagination from 'fuxsto-design/pagination'
import { Message } from 'fuxsto-design/message'
import {
  fetchTags,
  searchImages,
  type Repository,
  type TagInfo,
} from '~/utils/api'
import { copyText, errorMessage, formatArchs, formatNumber, formatSize, formatTimeAgo } from '~/utils/format'

useHead({ title: '镜像搜索 · li-gh-proxy' })

interface RepoView {
  raw: Repository
  displayName: string
  namespace: string
  name: string
  fullRepoName: string
}

const route = useRoute()
const router = useRouter()

const query = ref('')
const searching = ref(false)
const searchError = ref('')
const results = ref<RepoView[]>([])
const resultCount = ref(0)
const resultsPage = ref(1)
const pageSize = 25

const selected = ref<RepoView | null>(null)
const tagsLoading = ref(false)
const tagsError = ref('')
const tags = ref<TagInfo[]>([])
const tagFilter = ref('')
const tagsPage = ref(1)
const tagsHasMore = ref(false)
const TAG_PAGE_SIZE = 100

const hasResults = computed(() => results.value.length > 0)
const totalPages = computed(() => Math.max(1, Math.ceil(resultCount.value / pageSize)))

const tagsTotal = computed(() =>
  tagsHasMore.value ? (tagsPage.value + 1) * TAG_PAGE_SIZE : tagsPage.value * TAG_PAGE_SIZE,
)

const filteredTags = computed(() => {
  const q = tagFilter.value.trim().toLowerCase()
  if (!q) return tags.value
  const exact: TagInfo[] = []
  const starts: TagInfo[] = []
  const includes: TagInfo[] = []
  for (const tag of tags.value) {
    const name = tag.name.toLowerCase()
    if (name === q) exact.push(tag)
    else if (name.startsWith(q)) starts.push(tag)
    else if (name.includes(q)) includes.push(tag)
  }
  return [...exact, ...starts, ...includes]
})

const displayTags = computed(() =>
  filteredTags.value.map((tag) => ({
    tag,
    archs: formatArchs(tag.images),
    size: formatSize(tag.full_size),
  })),
)

function toRepoView(item: Repository): RepoView | null {
  const rawName = item.repo_name || ''
  const namespace =
    item.namespace ||
    (item.is_official ? 'library' : rawName.includes('/') ? rawName.split('/')[0] : '')
  const name = rawName.replace(/^library\//, '').includes('/')
    ? rawName.split('/').pop() || ''
    : rawName.replace(/^library\//, '')

  if (!namespace || !name) return null

  const displayName = item.is_official
    ? name
    : item.namespace
      ? `${item.namespace}/${name}`
      : rawName.includes('/')
        ? rawName
        : `${namespace}/${name}`

  return {
    raw: item,
    displayName,
    namespace,
    name,
    fullRepoName: item.is_official ? name : `${namespace}/${name}`,
  }
}

async function runSearch(q: string, page = 1) {
  const trimmed = q.trim()
  if (!trimmed) {
    searchError.value = '请输入搜索关键词'
    return
  }

  searching.value = true
  searchError.value = ''
  results.value = []
  selected.value = null
  tags.value = []
  tagsError.value = ''
  tagFilter.value = ''

  try {
    let searchQuery = trimmed
    let targetRepo = ''
    if (trimmed.includes('/')) {
      const [ns = ''] = trimmed.split('/')
      searchQuery = ns
      targetRepo = trimmed.toLowerCase()
    }

    const data = await searchImages(searchQuery, page, pageSize)
    const views = (data.results || [])
      .map(toRepoView)
      .filter((v): v is RepoView => v !== null)

    views.sort((a, b) => {
      if (targetRepo) {
        const aMatch =
          a.displayName.toLowerCase() === targetRepo ||
          a.fullRepoName.toLowerCase() === targetRepo
        const bMatch =
          b.displayName.toLowerCase() === targetRepo ||
          b.fullRepoName.toLowerCase() === targetRepo
        if (aMatch && !bMatch) return -1
        if (!aMatch && bMatch) return 1
      }
      if (!!a.raw.is_official !== !!b.raw.is_official) {
        return a.raw.is_official ? -1 : 1
      }
      return (b.raw.pull_count || 0) - (a.raw.pull_count || 0)
    })

    results.value = views
    resultCount.value = data.count ?? views.length
    resultsPage.value = page
    if (views.length === 0) searchError.value = '未找到相关镜像'
  } catch (e) {
    searchError.value = errorMessage(e, '搜索失败')
  } finally {
    searching.value = false
  }
}

async function onSearch() {
  const q = query.value.trim()
  await router.replace({ path: '/search', query: q ? { q } : {} })
  await runSearch(q, 1)
}

async function onResultsPageChange(page: number) {
  if (searching.value) return
  await runSearch(query.value, page)
  window.scrollTo(0, 0)
}

async function loadTagPage(repo: RepoView, page: number) {
  selected.value = repo
  tagsLoading.value = true
  tagsError.value = ''
  tagsPage.value = page
  if (page === 1) tagFilter.value = ''

  try {
    const data = await fetchTags(repo.namespace, repo.name, page, TAG_PAGE_SIZE)
    tags.value = data.tags || []
    tagsHasMore.value = !!data.has_more
    if (tags.value.length === 0) tagsError.value = '该镜像暂无可用标签'
  } catch (e) {
    tags.value = []
    tagsError.value = errorMessage(e, '加载标签失败')
  } finally {
    tagsLoading.value = false
  }
}

async function onTagsPageChange(page: number) {
  if (!selected.value || tagsLoading.value) return
  await loadTagPage(selected.value, page)
}

function backToResults() {
  selected.value = null
  tags.value = []
  tagsError.value = ''
  tagFilter.value = ''
}

async function copyPull(tagName?: string) {
  if (!selected.value) return
  const image = tagName
    ? `${window.location.host}/${selected.value.fullRepoName}:${tagName}`
    : `${window.location.host}/${selected.value.fullRepoName}`
  const refName = `docker pull ${image}`
  const ok = await copyText(refName)
  if (ok) Message.success(`已复制 ${refName}`)
  else Message.error('复制失败')
}

watch(
  () => route.query.q,
  async (q) => {
    const next = typeof q === 'string' ? q : ''
    if (next === query.value) return
    query.value = next
    if (next) await runSearch(next, 1)
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <PageHero
      eyebrow="Docker Hub"
      title="镜像搜索"
      subtitle="检索官方与社区镜像，查看标签与架构，一键复制拉取命令。"
    />

    <Transition name="fade" mode="out-in">
      <div v-if="!selected" key="search" class="mx-auto max-w-3xl space-y-6">
        <div class="flex flex-col gap-3 sm:flex-row">
          <Input
            v-model="query"
            class="sm:flex-1"
            placeholder="例如 nginx、redis、library/ubuntu"
            @keydown.enter="onSearch"
          />
          <Button :loading="searching" :disabled="searching" @click="onSearch">
            <Search v-if="!searching" class="size-4" />
            {{ searching ? '搜索中...' : '搜索' }}
          </Button>
        </div>

        <p v-if="searchError && !searching" class="text-center text-destructive">
          {{ searchError }}
        </p>

        <div v-if="searching" class="space-y-3 pt-2">
          <Skeleton :rows="3" :height="64" round />
        </div>

        <div v-else-if="hasResults" class="space-y-2">
          <p class="text-center text-muted-foreground">
            共 {{ resultCount }} 条结果
            <template v-if="totalPages > 1"> · 第 {{ resultsPage }} / {{ totalPages }} 页</template>
          </p>
          <div class="divide-y divide-border border-y border-border">
            <button
              v-for="item in results"
              :key="`${item.namespace}/${item.name}`"
              type="button"
              class="w-full py-4 text-left transition-colors duration-150 hover:text-primary"
              @click="loadTagPage(item, 1)"
            >
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <span class="text-base font-medium">{{ item.displayName }}</span>
                <Chip v-if="item.raw.is_official" variant="primary" size="sm">官方</Chip>
                <span v-if="item.raw.star_count" class="text-xs text-muted-foreground">
                  ★ {{ formatNumber(item.raw.star_count) }}
                </span>
                <span v-if="item.raw.pull_count" class="text-xs text-muted-foreground">
                  ⬇ {{ formatNumber(item.raw.pull_count) }}
                </span>
              </div>
              <p class="line-clamp-2 text-muted-foreground">
                {{ item.raw.short_description || '暂无描述' }}
              </p>
            </button>
          </div>
          <div v-if="totalPages > 1" class="flex justify-center pt-2">
            <Pagination
              v-model:current="resultsPage"
              :page-size="pageSize"
              :total="resultCount"
              :disabled="searching"
              @change="onResultsPageChange"
            />
          </div>
        </div>

        <Empty
          v-else-if="!searchError"
          title="暂无结果"
          description="输入关键词开始搜索 Docker Hub 镜像"
          class="pt-6"
        />
      </div>

      <div v-else key="tags" class="mx-auto max-w-3xl space-y-6">
        <button
          type="button"
          class="flex items-center gap-1 text-muted-foreground transition-colors hover:text-primary"
          @click="backToResults"
        >
          <ChevronLeft class="size-4" />
          返回搜索结果
        </button>

        <div class="space-y-2 text-center">
          <div class="flex flex-wrap items-center justify-center gap-2">
            <h2 class="text-2xl font-semibold tracking-tight sm:text-3xl">
              {{ selected.fullRepoName }}
            </h2>
            <Chip v-if="selected.raw.is_official" variant="primary" size="sm">官方</Chip>
          </div>
          <p class="text-base text-muted-foreground">
            {{ selected.raw.short_description || '暂无描述' }}
          </p>
        </div>

        <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
          <Input
            v-model="tagFilter"
            class="sm:flex-1"
            placeholder="筛选当前页标签..."
            clearable
          />
          <div class="flex items-center gap-1.5">
            <Button variant="outline" size="sm" @click="copyPull()">
              <Copy class="size-4" />
              复制
            </Button>
          </div>
        </div>

        <p v-if="tagsError" class="text-center text-destructive">{{ tagsError }}</p>
        <div v-else-if="tagsLoading" class="space-y-3">
          <Skeleton :rows="5" :height="56" round />
        </div>
        <Empty
          v-else-if="displayTags.length === 0"
          title="没有匹配的标签"
          variant="dashed"
          size="sm"
        />
        <div v-else class="divide-y divide-border border-y border-border">
          <div
            v-for="{ tag, archs, size } in displayTags"
            :key="tag.name"
            class="flex items-start justify-between gap-3 py-4"
          >
            <div class="min-w-0 space-y-1.5">
              <p class="truncate text-base font-medium">{{ tag.name }}</p>
              <p class="text-xs text-muted-foreground">
                <template v-if="size">{{ size }} · </template>
                {{ formatTimeAgo(tag.last_updated) }}
              </p>
              <div v-if="archs.length" class="flex flex-wrap gap-1.5">
                <Chip v-for="arch in archs" :key="arch" variant="secondary" size="sm" class="font-mono">
                  {{ arch }}
                </Chip>
              </div>
            </div>
            <Button variant="outline" size="sm" class="shrink-0" @click="copyPull(tag.name)">
              <Copy class="size-4" />
              复制
            </Button>
          </div>
        </div>

        <div v-if="!tagsError && (tags.length > 0 || tagsPage > 1)" class="flex justify-center">
          <Pagination
            v-model:current="tagsPage"
            :page-size="TAG_PAGE_SIZE"
            :total="tagsTotal"
            :disabled="tagsLoading"
            @change="onTagsPageChange"
          />
        </div>
      </div>
    </Transition>
  </div>
</template>
