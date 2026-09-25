<script setup lang="ts">
import Button from 'fuxsto-design/button'
import Input from 'fuxsto-design/input'
import Textarea from 'fuxsto-design/textarea'
import Switch from 'fuxsto-design/switch'
import Card from 'fuxsto-design/card'
import Alert from 'fuxsto-design/alert'
import { Message } from 'fuxsto-design/message'
import {
  fetchImageInfo,
  prepareBatchDownload,
  prepareSingleDownload,
  triggerDownload,
} from '~/utils/api'
import { errorMessage } from '~/utils/format'

useHead({ title: '离线镜像下载 · li-gh-proxy' })

const singleImage = ref('')
const singlePlatform = ref('linux/amd64')
const singleCompressed = ref(true)
const singleError = ref('')
const singleLoading = ref(false)

const batchText = ref('')
const batchPlatform = ref('linux/amd64')
const batchCompressed = ref(true)
const batchError = ref('')
const batchLoading = ref(false)

async function preflight(images: string[]) {
  for (const image of [...new Set(images)]) {
    await fetchImageInfo(image)
  }
}

async function onSingleSubmit() {
  singleError.value = ''
  const image = singleImage.value.trim()
  if (!image) {
    singleError.value = '请输入镜像名称'
    return
  }

  singleLoading.value = true
  try {
    await preflight([image])
    const data = await prepareSingleDownload({
      image,
      platform: singlePlatform.value,
      compressed: singleCompressed.value,
    })
    if (!data.download_url) throw new Error('下载地址生成失败')
    triggerDownload(data.download_url)
    const platformText = singlePlatform.value.trim() ? ` (${singlePlatform.value.trim()})` : ''
    Message.success(`开始下载 ${image}${platformText}`)
  } catch (e) {
    singleError.value = errorMessage(e, '下载失败')
  } finally {
    singleLoading.value = false
  }
}

async function onBatchSubmit() {
  batchError.value = ''
  const images = batchText.value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'))

  if (images.length === 0) {
    batchError.value = '请输入镜像列表'
    return
  }

  batchLoading.value = true
  try {
    await preflight(images)
    const data = await prepareBatchDownload({
      images,
      platform: batchPlatform.value,
      useCompressedLayers: batchCompressed.value,
    })
    if (!data.download_url) throw new Error('下载地址生成失败')
    triggerDownload(data.download_url)
    Message.success(`开始下载 ${images.length} 个镜像`)
  } catch (e) {
    batchError.value = errorMessage(e, '下载失败')
  } finally {
    batchLoading.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-3xl">
    <PageHero
      eyebrow="Offline Image"
      title="离线镜像"
      subtitle="流式下载，兼容 docker load，支持多架构。"
    />

    <Card padding="lg" class="field-block">
      <template #header>
        <h2 class="text-center text-sm font-semibold tracking-[0.16em] text-muted-foreground uppercase">
          单镜像
        </h2>
      </template>

      <Alert v-if="singleError" type="error" :title="singleError" />

      <label class="block space-y-1.5">
        <span>镜像名称</span>
        <Input v-model="singleImage" placeholder="nginx 或 user/app:tag" />
      </label>
      <label class="block space-y-1.5">
        <span>目标架构（可选）</span>
        <Input v-model="singlePlatform" placeholder="linux/amd64" />
      </label>
      <div class="flex items-center justify-between py-1">
        <span>压缩层</span>
        <Switch v-model="singleCompressed" />
      </div>
      <Button class="w-full" :loading="singleLoading" :disabled="singleLoading" @click="onSingleSubmit">
        {{ singleLoading ? '准备中...' : '立即下载' }}
      </Button>
    </Card>

    <Card padding="lg" class="section-gap field-block">
      <template #header>
        <h2 class="text-center text-sm font-semibold tracking-[0.16em] text-muted-foreground uppercase">
          批量下载
        </h2>
      </template>

      <Alert v-if="batchError" type="error" :title="batchError" />

      <label class="block space-y-1.5">
        <span>镜像列表</span>
        <Textarea
          v-model="batchText"
          :rows="5"
          placeholder="alpine&#10;redis:alpine&#10;user/app:1.0"
        />
      </label>
      <label class="block space-y-1.5">
        <span>目标架构（可选）</span>
        <Input v-model="batchPlatform" placeholder="linux/amd64" />
      </label>
      <div class="flex items-center justify-between py-1">
        <span>压缩层</span>
        <Switch v-model="batchCompressed" />
      </div>
      <Button class="w-full" :loading="batchLoading" :disabled="batchLoading" @click="onBatchSubmit">
        {{ batchLoading ? '准备中...' : '批量下载' }}
      </Button>
    </Card>
  </div>
</template>
