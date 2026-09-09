<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { deleteUser, listUsers, updateUser } from '@/api/admin'
import type { PageData, UserInfo } from '@/api/types'
import { dialog, message } from '@/utils/notify'
import { formatDate, formatSize } from '@/utils/format'

const page = ref(1)
const pageSize = 10
const keyword = ref('')
const loading = ref(false)
const data = ref<PageData<UserInfo>>({ list: [], total: 0 })

async function load() {
  loading.value = true
  try {
    const res = await listUsers({ page: page.value, page_size: pageSize, keyword: keyword.value.trim() })
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

function toggleStatus(row: UserInfo) {
  const next = row.status === 1 ? 0 : 1
  updateUser(row.id, { status: next })
    .then(() => {
      message.success(next === 1 ? '已启用该用户' : '已禁用该用户')
      load()
    })
    .catch(() => {
      /* 拦截器已提示 */
    })
}

function quotaPct(row: UserInfo): number {
  if (!row.quota_max) return 0
  return Math.min(100, Math.round((row.quota_used / row.quota_max) * 100))
}

/* 配额 */
const quotaVisible = ref(false)
const quotaTarget = ref<UserInfo | null>(null)
const quotaValue = ref<number | null>(null)
const quotaLoading = ref(false)

function openQuota(row: UserInfo) {
  quotaTarget.value = row
  quotaValue.value = Math.round((row.quota_max || 0) / (1024 * 1024))
  quotaVisible.value = true
}

async function onQuota() {
  if (!quotaTarget.value || quotaValue.value == null || quotaValue.value < 0) {
    message.warning('请输入有效的配额数值（MB）')
    return
  }
  quotaLoading.value = true
  try {
    await updateUser(quotaTarget.value.id, { quota_max: Math.round(quotaValue.value * 1024 * 1024) })
    message.success('配额更新成功')
    quotaVisible.value = false
    load()
  } catch {
    /* ignore */
  } finally {
    quotaLoading.value = false
  }
}

function onDelete(row: UserInfo) {
  dialog.warning({
    title: '删除用户',
    content: `确定要删除用户“${row.username}”吗？其名下文件将一并删除，此操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteUser(row.id)
        message.success('用户已删除')
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
      <div class="page-title">用户管理</div>
      <div class="toolbar-right">
        <n-input v-model:value="keyword" placeholder="搜索用户名 / 邮箱" clearable style="width: 220px" @keyup.enter="onSearch" />
        <n-button type="primary" @click="onSearch">搜索</n-button>
        <n-button @click="load">刷新</n-button>
      </div>
    </div>

    <n-spin :show="loading">
      <table class="data-table">
        <thead>
          <tr>
            <th style="width: 64px">ID</th>
            <th>用户名</th>
            <th>邮箱</th>
            <th style="width: 90px">角色</th>
            <th style="width: 190px">空间配额</th>
            <th style="width: 76px">状态</th>
            <th style="width: 160px">注册时间</th>
            <th style="width: 210px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in data.list" :key="row.id">
            <td>{{ row.id }}</td>
            <td>{{ row.username }}</td>
            <td>{{ row.email || '-' }}</td>
            <td>
              <n-tag :type="row.role === 1 ? 'primary' : 'default'" size="small" :bordered="false">
                {{ row.role === 1 ? '管理员' : '普通用户' }}
              </n-tag>
            </td>
            <td>
              <div class="quota-cell">
                <n-progress
                  type="line"
                  :percentage="quotaPct(row)"
                  :height="4"
                  :border-radius="2"
                  :show-indicator="false"
                  :status="quotaPct(row) >= 90 ? 'error' : 'default'"
                />
                <span class="quota-mini">{{ formatSize(row.quota_used) }} / {{ formatSize(row.quota_max) }}</span>
              </div>
            </td>
            <td>
              <n-tag :type="row.status === 1 ? 'success' : 'error'" size="small" :bordered="false">
                {{ row.status === 1 ? '正常' : '禁用' }}
              </n-tag>
            </td>
            <td>{{ formatDate(row.created_at) }}</td>
            <td>
              <n-space :size="4">
                <n-button size="tiny" quaternary @click="toggleStatus(row)">{{ row.status === 1 ? '禁用' : '启用' }}</n-button>
                <n-button size="tiny" quaternary @click="openQuota(row)">设置配额</n-button>
                <n-button size="tiny" quaternary type="error" @click="onDelete(row)">删除</n-button>
              </n-space>
            </td>
          </tr>
        </tbody>
      </table>
    </n-spin>

    <div class="page-pagination">
      <n-pagination v-model:page="page" :item-count="data.total" :page-size="pageSize" @update:page="load" />
    </div>

    <n-modal v-model:show="quotaVisible" preset="card" title="设置空间配额" style="width: 400px" :bordered="false">
      <div class="quota-form">
        <n-input-number v-model:value="quotaValue" :min="0" :step="1024" style="width: 100%" />
        <p class="quota-hint">单位：MB（1 GB = 1024 MB）</p>
      </div>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="quotaVisible = false">取消</n-button>
          <n-button type="primary" :loading="quotaLoading" @click="onQuota">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
