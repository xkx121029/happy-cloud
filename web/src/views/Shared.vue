<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowBackOutline, DownloadOutline } from '@vicons/ionicons5'
import { listShared } from '@/api/files'
import type { SharedFileItem } from '@/api/types'
import FileIcon from '@/components/FileIcon.vue'
import { downloadSharedFile } from '@/utils/download'
import { formatDate, formatSize } from '@/utils/format'

const router = useRouter()
const loading = ref(false)
const files = ref<SharedFileItem[]>([])

async function load() {
  loading.value = true
  try {
    files.value = await listShared()
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function onDownload(f: SharedFileItem) {
  if (f.type !== 1) return
  try {
    await downloadSharedFile(f.id, f.name)
  } catch {
    /* 拦截器已提示 */
  }
}

onMounted(load)
</script>

<template>
  <div class="home-page">
    <header class="home-header">
      <div class="home-header-inner">
        <div class="home-brand">
          <n-button quaternary circle size="small" title="返回我的网盘" @click="router.push('/')">
            <template #icon><n-icon><ArrowBackOutline /></n-icon></template>
          </n-button>
          <div class="brand-mark">HC</div>
          <div>
            <div class="brand-name">共享目录</div>
            <div class="shared-subtitle">全体用户共享的文件空间</div>
          </div>
        </div>
      </div>
    </header>

    <main class="home-main">
      <div class="toolbar">
        <div class="page-title">公共共享文件</div>
        <div class="toolbar-right">
          <n-button :loading="loading" @click="load">
            <template #icon><n-icon><DownloadOutline /></n-icon></template>
            刷新
          </n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div class="file-list-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>文件名</th>
                <th style="width: 110px">大小</th>
                <th style="width: 140px">共享者</th>
                <th style="width: 170px">共享时间</th>
                <th style="width: 100px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="f in files" :key="f.id">
                <td>
                  <div class="cell-name">
                    <FileIcon :file="f" :size="22" />
                    <span :title="f.name">{{ f.name }}</span>
                  </div>
                </td>
                <td>{{ formatSize(f.size) }}</td>
                <td>{{ f.username || '-' }}</td>
                <td>{{ formatDate(f.created_at) }}</td>
                <td>
                  <n-button size="tiny" quaternary type="primary" @click="onDownload(f)">下载</n-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <n-empty v-if="!loading && files.length === 0" description="暂无共享文件，去网盘中共享文件到公共目录" style="padding: 60px 0" />
      </n-spin>
    </main>
  </div>
</template>
