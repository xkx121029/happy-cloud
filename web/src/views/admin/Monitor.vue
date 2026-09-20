<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchStats } from '@/api/admin'
import type { AdminStats } from '@/api/types'
import { formatSize } from '@/utils/format'

const stats = ref<AdminStats | null>(null)
const loading = ref(false)
const autoRefresh = ref(true)
let timer: ReturnType<typeof setInterval> | null = null

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

function formatUptime(sec: number): string {
  if (!sec) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分钟`
}

function onAutoRefresh(v: boolean) {
  autoRefresh.value = v
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  if (v) {
    timer = setInterval(load, 10 * 1000)
  }
}

onMounted(() => {
  load()
  if (autoRefresh.value) timer = setInterval(load, 10 * 1000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <div class="page-toolbar">
      <div class="page-title">系统监控</div>
      <div class="toolbar-right">
        <div class="auto-refresh">
          <span>自动刷新</span>
          <n-switch :value="autoRefresh" @update:value="onAutoRefresh" size="small" />
        </div>
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

    <n-card size="small" title="进程资源" style="margin-top: 12px">
      <n-grid :cols="4" :x-gap="12" responsive="screen" item-responsive>
        <n-grid-item span="4 s:2 m:1">
          <div class="res-label">协程数</div>
          <div class="res-value">{{ stats?.resource?.goroutines ?? '-' }}</div>
        </n-grid-item>
        <n-grid-item span="4 s:2 m:1">
          <div class="res-label">内存占用</div>
          <div class="res-value">{{ stats?.resource ? formatSize(stats.resource.mem_alloc) : '-' }}</div>
        </n-grid-item>
        <n-grid-item span="4 s:2 m:1">
          <div class="res-label">系统内存</div>
          <div class="res-value">{{ stats?.resource ? formatSize(stats.resource.mem_sys) : '-' }}</div>
        </n-grid-item>
        <n-grid-item span="4 s:2 m:1">
          <div class="res-label">运行时长</div>
          <div class="res-value">{{ stats?.resource ? formatUptime(stats.resource.uptime) : '-' }}</div>
        </n-grid-item>
      </n-grid>
    </n-card>
  </div>
</template>

<style scoped>
.auto-refresh {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-right: 12px;
  color: var(--text-color-2, #6b7280);
  font-size: 13px;
}
.res-label {
  font-size: 13px;
  color: var(--text-color-2, #6b7280);
}
.res-value {
  font-size: 18px;
  font-weight: 600;
  margin-top: 4px;
}
</style>
