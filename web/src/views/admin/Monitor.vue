<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { fetchStats } from '@/api/admin'
import type { AdminStats } from '@/api/types'
import { formatSize } from '@/utils/format'

const stats = ref<AdminStats | null>(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    stats.value = await fetchStats()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-toolbar">
      <div class="page-title">系统监控</div>
      <div class="toolbar-right">
        <n-button :loading="loading" @click="load">刷新</n-button>
      </div>
    </div>

    <n-grid :cols="5" :x-gap="12" :y-gap="12" responsive="screen" item-responsive>
      <n-grid-item span="5 m:1">
        <n-card size="small" class="stat-card">
          <div class="stat-label">用户总数</div>
          <div class="stat-value">{{ stats?.user_count ?? '-' }}</div>
        </n-card>
      </n-grid-item>
      <n-grid-item span="5 m:1">
        <n-card size="small" class="stat-card">
          <div class="stat-label">文件总数</div>
          <div class="stat-value">{{ stats?.file_count ?? '-' }}</div>
        </n-card>
      </n-grid-item>
      <n-grid-item span="5 m:1">
        <n-card size="small" class="stat-card">
          <div class="stat-label">存储用量</div>
          <div class="stat-value">{{ stats ? formatSize(stats.storage_used) : '-' }}</div>
        </n-card>
      </n-grid-item>
      <n-grid-item span="5 m:1">
        <n-card size="small" class="stat-card">
          <div class="stat-label">今日上传</div>
          <div class="stat-value">{{ stats?.today_uploads ?? '-' }}</div>
        </n-card>
      </n-grid-item>
      <n-grid-item span="5 m:1">
        <n-card size="small" class="stat-card">
          <div class="stat-label">在线用户</div>
          <div class="stat-value">{{ stats?.online_users ?? '-' }}</div>
        </n-card>
      </n-grid-item>
    </n-grid>
  </div>
</template>
