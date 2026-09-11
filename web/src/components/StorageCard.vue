<script setup lang="ts">
import { computed } from 'vue'
import { formatSize } from '@/utils/format'

export interface StorageOverviewData {
  quota_max: number
  quota_used: number
  categories: { image: number; video: number; audio: number; doc: number; other: number }
}

const props = defineProps<{ data: StorageOverviewData | null }>()

const percent = computed(() => {
  const d = props.data
  if (!d || !d.quota_max) return 0
  return Math.min(100, Math.round((d.quota_used / d.quota_max) * 100))
})

const categories = computed(() => {
  const d = props.data
  if (!d) return []
  const cats = d.categories ?? { image: 0, video: 0, audio: 0, doc: 0, other: 0 }
  return [
    { label: '图片', key: 'image', size: cats.image, color: '#38bdf8' },
    { label: '视频', key: 'video', size: cats.video, color: '#a78bfa' },
    { label: '音频', key: 'audio', size: cats.audio, color: '#34d399' },
    { label: '文档', key: 'doc', size: cats.doc, color: '#fbbf24' },
    { label: '其他', key: 'other', size: cats.other, color: '#94a3b8' }
  ].filter((c) => c.size > 0)
})

const colorStatus = computed(() => {
  if (percent.value >= 90) return 'error'
  if (percent.value >= 70) return 'warning'
  return 'success'
})
</script>

<template>
  <div class="storage-card">
    <div class="storage-card-title">存储空间</div>
    <div class="storage-main">
      <n-progress type="circle" :percentage="percent" :height="88" :stroke-width="10" :status="colorStatus">
        <div class="storage-percent">{{ percent }}%</div>
      </n-progress>
      <div class="storage-quota">
        <div class="storage-used">{{ formatSize(data?.quota_used ?? 0) }}</div>
        <div class="storage-max">/ {{ formatSize(data?.quota_max ?? 0) }}</div>
      </div>
    </div>
    <div v-if="categories.length" class="storage-cats">
      <div v-for="c in categories" :key="c.key" class="storage-cat">
        <span class="storage-cat-dot" :style="{ background: c.color }"></span>
        <span class="storage-cat-label">{{ c.label }}</span>
        <span class="storage-cat-size">{{ formatSize(c.size) }}</span>
      </div>
    </div>
    <div v-else class="storage-empty">暂无已用空间</div>
  </div>
</template>
