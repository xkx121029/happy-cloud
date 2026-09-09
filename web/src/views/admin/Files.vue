<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { forceDeleteFile, listAdminFiles } from '@/api/admin'
import type { AdminFileItem, PageData } from '@/api/types'
import { dialog, message } from '@/utils/notify'
import { formatDate, formatSize } from '@/utils/format'

const page = ref(1)
const pageSize = 10
const keyword = ref('')
const loading = ref(false)
const data = ref<PageData<AdminFileItem>>({ list: [], total: 0 })

async function load() {
  loading.value = true
  try {
    const res = await listAdminFiles({ page: page.value, page_size: pageSize, keyword: keyword.value.trim() })
    data.value = Array.isArray(res) ? { list: res, total: res.length } : res
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function onSearch() {
  page.value = 1
  load()
}

function onDelete(row: AdminFileItem) {
  dialog.warning({
    title: '强制删除文件',
    content: `确定要强制删除“${row.name}”吗？该操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await forceDeleteFile(row.id)
        message.success('文件已删除')
        load()
      } catch {
        /* ignore */
      }
    }
  })
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-toolbar">
      <div class="page-title">文件管理</div>
      <div class="toolbar-right">
        <n-input v-model:value="keyword" placeholder="按文件名搜索全站文件" clearable style="width: 240px" @keyup.enter="onSearch" />
        <n-button type="primary" @click="onSearch">搜索</n-button>
        <n-button @click="load">刷新</n-button>
      </div>
    </div>

    <n-spin :show="loading">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width: 64px">ID</th>
            <th>文件名</th>
            <th style="width: 90px">类型</th>
            <th style="width: 110px">大小</th>
            <th style="width: 120px">所属用户</th>
            <th style="width: 160px">创建时间</th>
            <th style="width: 100px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in data.list" :key="row.id">
            <td>{{ row.id }}</td>
            <td :title="row.name" style="max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ row.name }}</td>
            <td>
              <n-tag :type="row.type === 0 ? 'primary' : 'default'" size="small" :bordered="false">
                {{ row.type === 0 ? '文件夹' : '文件' }}
              </n-tag>
            </td>
            <td>{{ row.type === 0 ? '-' : formatSize(row.size) }}</td>
            <td>{{ row.username || '-' }}</td>
            <td>{{ formatDate(row.created_at) }}</td>
            <td>
              <n-button size="tiny" quaternary type="error" @click="onDelete(row)">强制删除</n-button>
            </td>
          </tr>
        </tbody>
      </table>
    </n-spin>

    <div class="page-pagination">
      <n-pagination v-model:page="page" :item-count="data.total" :page-size="pageSize" @update:page="load" />
    </div>
  </div>
</template>
