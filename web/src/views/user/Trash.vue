<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowBackOutline,
  GridOutline,
  ListOutline,
  RefreshOutline,
  SearchOutline,
  TrashBinOutline
} from '@vicons/ionicons5'
import { clearTrash, deletePermanent, listTrash, restoreFiles } from '@/api/files'
import type { FileItem } from '@/api/types'
import FileIcon from '@/components/FileIcon.vue'
import { dialog, message } from '@/utils/notify'
import { formatDate, formatSize } from '@/utils/format'

const router = useRouter()

const files = ref<FileItem[]>([])
const loading = ref(false)
const viewMode = ref<'grid' | 'list'>('list')
const keyword = ref('')

/* 选择模式 */
const selecting = ref(false)
const selected = ref<Set<number>>(new Set())

const allChecked = computed(() => files.value.length > 0 && selected.value.size === files.value.length)
const selectedCount = computed(() => selected.value.size)
const selectedIds = computed(() => Array.from(selected.value))

async function load() {
  loading.value = true
  try {
    const data = await listTrash(keyword.value.trim())
    files.value = Array.isArray(data) ? data : data.items ?? []
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function toggleAll() {
  if (allChecked.value) selected.value.clear()
  else selected.value = new Set(files.value.map((f) => f.id))
}

function toggleOne(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
}

async function onRestore(id?: number) {
  const ids = id != null ? [id] : selectedIds.value
  if (!ids.length) return message.warning('请先选择文件')
  try {
    await restoreFiles(ids)
    message.success('已恢复所选文件')
    selected.value.clear()
    load()
  } catch {
    /* 拦截器已提示 */
  }
}

async function onPurge(id?: number) {
  const ids = id != null ? [id] : selectedIds.value
  if (!ids.length) return message.warning('请先选择文件')
  dialog.warning({
    title: '彻底删除',
    content: '彻底删除后将无法恢复，且会永久释放空间。确定继续吗？',
    positiveText: '彻底删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deletePermanent(ids)
        message.success('已彻底删除')
        selected.value.clear()
        load()
      } catch {
        /* 拦截器已提示 */
      }
    }
  })
}

function onClearAll() {
  if (!files.value.length) return message.info('回收站是空的')
  dialog.warning({
    title: '清空回收站',
    content: '将永久删除回收站内全部文件，且无法恢复。确定清空吗？',
    positiveText: '清空',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await clearTrash()
        message.success('回收站已清空')
        load()
      } catch {
        /* 拦截器已提示 */
      }
    }
  })
}

onMounted(load)
</script>

<template>
  <div class="home-page">
    <header class="home-header">
      <div class="home-header-inner">
        <div class="home-brand">
          <div class="brand-mark">HC</div>
          <span class="brand-name">回收站</span>
        </div>
        <div class="header-right">
          <n-button quaternary @click="router.push('/')">
            <template #icon><n-icon><ArrowBackOutline /></n-icon></template>
            返回我的网盘
          </n-button>
          <n-button type="error" quaternary :disabled="!files.length" @click="onClearAll">
            <template #icon><n-icon><TrashBinOutline /></n-icon></template>
            清空回收站
          </n-button>
        </div>
      </div>
    </header>

    <main class="home-main">
      <div class="toolbar">
        <div class="toolbar-info">
          <span class="page-title">回收站</span>
          <!-- L9 修复：后端无 30 天自动清理机制，改为如实说明回收站仍占空间 -->
          <span class="shared-subtitle">共 {{ files.length }} 项，回收站中的文件仍占用存储空间，可恢复或彻底删除以释放空间</span>
        </div>
        <div class="toolbar-right">
          <n-input v-model:value="keyword" placeholder="搜索回收站" clearable style="width: 200px" @keyup.enter="load">
            <template #prefix><n-icon><SearchOutline /></n-icon></template>
          </n-input>
          <n-button-group>
            <n-button :type="viewMode === 'grid' ? 'primary' : 'default'" size="small" @click="viewMode = 'grid'">
              <template #icon><n-icon><GridOutline /></n-icon></template>
            </n-button>
            <n-button :type="viewMode === 'list' ? 'primary' : 'default'" size="small" @click="viewMode = 'list'">
              <template #icon><n-icon><ListOutline /></n-icon></template>
            </n-button>
          </n-button-group>
          <n-button quaternary circle size="small" title="刷新" @click="load">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
          </n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div v-if="viewMode === 'grid'" class="file-grid">
          <div v-for="f in files" :key="f.id" class="file-card trash-card" :class="{ selected: selected.has(f.id) }" @click="toggleOne(f.id)">
            <div class="file-card-select" @click.stop>
              <n-checkbox :checked="selected.has(f.id)" @click.stop="toggleOne(f.id)" />
            </div>
            <div class="file-card-icon"><FileIcon :file="f" :size="42" /></div>
            <div class="file-card-name" :title="f.name">{{ f.name }}</div>
            <div class="file-card-meta">{{ f.type === 0 ? '文件夹' : formatSize(f.size) }}</div>
            <div class="trash-card-date">{{ formatDate(f.updated_at ?? f.created_at) }}</div>
            <div class="file-card-actions" @click.stop>
              <n-button quaternary circle size="tiny" type="primary" title="恢复" @click="onRestore(f.id)">
                <template #icon><n-icon><RefreshOutline /></n-icon></template>
              </n-button>
              <n-button quaternary circle size="tiny" type="error" title="彻底删除" @click="onPurge(f.id)">
                <template #icon><n-icon><TrashBinOutline /></n-icon></template>
              </n-button>
            </div>
          </div>
        </div>

        <div v-else class="file-list-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th style="width: 40px"><n-checkbox :checked="allChecked" @update:checked="toggleAll" /></th>
                <th>名称</th>
                <th style="width: 100px">大小</th>
                <th style="width: 150px">删除时间</th>
                <th style="width: 150px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="f in files" :key="f.id" :class="{ 'row-selected': selected.has(f.id) }">
                <td><n-checkbox :checked="selected.has(f.id)" @update:checked="toggleOne(f.id)" /></td>
                <td>
                  <div class="cell-name">
                    <FileIcon :file="f" :size="22" />
                    <span :title="f.name">{{ f.name }}</span>
                  </div>
                </td>
                <td>{{ f.type === 0 ? '-' : formatSize(f.size) }}</td>
                <td>{{ formatDate(f.updated_at ?? f.created_at) }}</td>
                <td>
                  <n-space :size="4">
                    <n-button quaternary size="tiny" type="primary" @click="onRestore(f.id)">恢复</n-button>
                    <n-button quaternary size="tiny" type="error" @click="onPurge(f.id)">彻底删除</n-button>
                  </n-space>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <n-empty v-if="!loading && files.length === 0" description="回收站是空的" style="padding: 60px 0" />
      </n-spin>

      <!-- 批量操作栏 -->
      <div v-if="selectedCount > 0" class="batch-bar">
        <span class="batch-count">已选择 {{ selectedCount }} 项</span>
        <n-space :size="8">
          <n-button size="small" type="primary" @click="onRestore()">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
            恢复
          </n-button>
          <n-button size="small" type="error" @click="onPurge()">
            <template #icon><n-icon><TrashBinOutline /></n-icon></template>
            彻底删除
          </n-button>
          <n-button size="small" @click="selected.clear()">取消选择</n-button>
        </n-space>
      </div>
    </main>
  </div>
</template>
