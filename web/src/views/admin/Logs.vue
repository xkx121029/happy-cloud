<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listLogs } from '@/api/admin'
import type { AdminLog, PageData } from '@/api/types'
import { formatDate } from '@/utils/format'

const page = ref(1)
const pageSize = 20
const loading = ref(false)
const data = ref<PageData<AdminLog>>({ list: [], total: 0 })

async function load() {
  loading.value = true
  try {
    const res = await listLogs({ page: page.value, page_size: pageSize })
    data.value = Array.isArray(res) ? { list: res, total: res.length } : res
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
      <div class="page-title">操作日志</div>
      <div class="toolbar-right">
        <n-button @click="load">刷新</n-button>
      </div>
    </div>

    <n-spin :show="loading">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width: 64px">ID</th>
            <th style="width: 160px">用户</th>
            <th style="width: 160px">操作</th>
            <th>详情</th>
            <th style="width: 170px">时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in data.list" :key="row.id">
            <td>{{ row.id }}</td>
            <td>{{ row.username || `#${row.user_id}` }}</td>
            <td>{{ row.action }}</td>
            <td :title="row.detail" style="max-width: 420px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ row.detail || '-' }}</td>
            <td>{{ formatDate(row.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </n-spin>

    <div class="page-pagination">
      <n-pagination v-model:page="page" :item-count="data.total" :page-size="pageSize" @update:page="load" />
    </div>
  </div>
</template>
