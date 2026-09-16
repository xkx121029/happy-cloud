<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ArrowUndoOutline,
  CheckmarkCircleOutline,
  CloudDoneOutline,
  FilterOutline,
  InformationCircleOutline,
  RefreshOutline,
  TrashBinOutline
} from '@vicons/ionicons5'
import { NButton, NCard, NEllipsis, NTag, NInput } from 'naive-ui'
import { useDialog, useMessage } from 'naive-ui'
import {
  fetchStorageIndex,
  listBlobs,
  fetchBlobRefs,
  verifyStorage
} from '@/api/admin'
import type { BlobItem, BlobRefItem } from '@/api/admin'
import type { DedupStats } from '@/api/types'
import { formatDate, formatSize } from '@/utils/format'
import { toList, type PageData } from '@/api/types'

const dialog = useDialog()
const message = useMessage()

/* ---------- 顶部指标 ---------- */
const stats = ref<DedupStats | null>(null)
const statsLoading = ref(false)
async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await fetchStorageIndex()
  } catch {
    /* ignore */
  } finally {
    statsLoading.value = false
  }
}

const s = computed<DedupStats>(() => stats.value ?? { blob_count: 0, physical_bytes: 0, logical_bytes: 0, saved_bytes: 0, ref_count_sum: 0, trash_only_count: 0, unbilled_count: 0, legacy_bytes: 0 })

/* ---------- 实体列表 ---------- */
const blobsPage = ref(1)
const blobsPageSize = ref(20)
const blobsKeyword = ref('')
const blobsData = ref<PageData<BlobItem>>({ total: 0, items: [], page: 1, page_size: 20 })
const blobsLoading = ref(false)

async function loadBlobs() {
  blobsLoading.value = true
  try {
    blobsData.value = await listBlobs({
      page: blobsPage.value,
      page_size: blobsPageSize.value,
      keyword: blobsKeyword.value.trim() || undefined
    })
  } catch {
    /* ignore */
  } finally {
    blobsLoading.value = false
  }
}

function onBlobPage(p: number) {
  blobsPage.value = p
  loadBlobs()
}

/* ---------- 实体引用 ---------- */
const refsVisible = ref(false)
const refsHash = ref('')
const refsData = ref<BlobRefItem[]>([])
const refsLoading = ref(false)

async function openRefs(hash: string) {
  refsHash.value = hash
  refsVisible.value = true
  refsLoading.value = true
  try {
    const r = await fetchBlobRefs(hash)
    refsData.value = r.refs ?? []
  } catch {
    /* ignore */
  } finally {
    refsLoading.value = false
  }
}

/* ---------- 校验/修复 ---------- */
const verifyApply = ref(false)
const verifyGC = ref(false)
const verifyLoading = ref(false)
const verifyReport = ref<Record<string, unknown> | null>(null)

async function runVerify() {
  verifyLoading.value = true
  verifyReport.value = null
  try {
    const rep = await verifyStorage({ apply: verifyApply.value, gc_orphans: verifyGC.value })
    verifyReport.value = (rep as any)?.data ?? rep
    if (verifyApply.value) {
      message.success('校验完成并已应用修复')
      await loadStats()
      await loadBlobs()
    } else {
      message.info('仅预览差异，未执行修改（勾选「应用修复」再确认以生效）')
    }
  } catch (e: any) {
    message.error(e?.message || '校验失败')
  } finally {
    verifyLoading.value = false
  }
}

function requestVerify() {
  dialog.warning({
    title: '确认存储校验',
    content: verifyApply.value
      ? '将执行修复（修正 ref_count / billed_user_id、清理事件孤立的实体文件）。建议先 dry-run 预览差异。'
      : '仅预览差异，不会修改任何数据。',
    positiveText: verifyApply.value ? '应用修复' : '仅预览',
    negativeText: '取消',
    onPositiveClick: runVerify
  })
}

onMounted(() => {
  loadStats()
  loadBlobs()
})
</script>

<template>
  <div class="storage-page">
    <n-card title="存储索引" :loading="statsLoading" size="small" class="mb">
      <template #header-extra>
        <n-button size="small" quaternary circle @click="loadStats">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>
        </n-button>
      </template>
      <n-grid :cols="6" :x-gap="12" :y-gap="12">
        <n-grid-item>
          <n-statistic label="实体数量">
            <template #number>
              <span class="stat-num">{{ s.blob_count ?? '—' }}</span>
            </template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item>
          <n-statistic label="物理占用">
            <template #number><span class="stat-num">{{ formatSize(s.physical_bytes ?? 0) }}</span></template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item>
          <n-statistic label="逻辑占用">
            <template #number><span class="stat-num">{{ formatSize(s.logical_bytes ?? 0) }}</span></template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item>
          <n-statistic label="去重节省" :trend="'up'">
            <template #number><span class="stat-num success">{{ formatSize(s.saved_bytes ?? 0) }}</span></template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item>
          <n-statistic label="总引用数">
            <template #number><span class="stat-num">{{ s.ref_count_sum ?? '—' }}</span></template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item>
          <n-statistic label="回收站独占">
            <template #number>
              <n-tag v-if="s.trash_only_count" size="tiny" type="warning">
                {{ s.trash_only_count }}
              </n-tag>
              <span v-else class="stat-num">0</span>
            </template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item v-if="s.legacy_bytes > 0">
          <n-statistic label="未迁移字节">
            <template #number>
              <n-tag size="tiny" type="error">
                {{ formatSize(s.legacy_bytes) }}
              </n-tag>
            </template>
          </n-statistic>
        </n-grid-item>
        <n-grid-item v-if="s.unbilled_count">
          <n-statistic label="无人计费（异常）">
            <template #number>
              <n-tag size="tiny" type="error">{{ s.unbilled_count }}</n-tag>
            </template>
          </n-statistic>
        </n-grid-item>
      </n-grid>
    </n-card>

    <n-card title="实体列表" :loading="blobsLoading" size="small" class="mb">
      <template #header-extra>
        <n-input
          v-model:value="blobsKeyword"
          placeholder="按 hash 搜索"
          clearable
          style="width: 260px"
          @keyup.enter="loadBlobs"
        >
          <template #prefix><n-icon><FilterOutline /></n-icon></template>
        </n-input>
        <n-button size="small" quaternary circle @click="loadBlobs">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>
        </n-button>
      </template>
      <table class="data-table">
        <thead>
          <tr>
            <th>Hash</th>
            <th style="width:100px">大小</th>
            <th style="width:80px">引用</th>
            <th style="width:120px">计费归属</th>
            <th style="width:90px">磁盘</th>
            <th style="width:160px">创建时间</th>
            <th style="width:120px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in toList(blobsData)" :key="b.hash">
            <td>
              <n-ellipsis style="max-width: 260px">
                <code class="hash-text">{{ b.hash }}</code>
              </n-ellipsis>
            </td>
            <td>{{ formatSize(b.size) }}</td>
            <td>
              <n-tag :type="b.ref_count > 1 ? 'success' : 'default'" size="tiny">
                {{ b.ref_count }}
              </n-tag>
            </td>
            <td>{{ b.billed_username || '—' }}</td>
            <td>
              <n-tag :type="b.on_disk ? 'success' : 'error'" size="tiny">
                {{ b.on_disk ? '在盘' : '缺失' }}
              </n-tag>
            </td>
            <td>{{ b.created_at }}</td>
            <td>
              <n-button size="tiny" @click="openRefs(b.hash)">引用</n-button>
            </td>
          </tr>
          <tr v-if="!blobsData.items?.length && !blobsLoading">
            <td colspan="7" class="empty-row">暂无实体（尚未上传文件或未迁移存量）</td>
          </tr>
        </tbody>
      </table>
      <div v-if="blobsData.total" class="pagination">
        <n-button
          size="small"
          :disabled="blobsPage <= 1"
          @click="onBlobPage(blobsPage - 1)"
        >上一页</n-button>
        <span class="page-info">第 {{ blobsPage }} 页 / 共 {{ Math.ceil(blobsData.total / blobsPageSize) }} 页</span>
        <n-button
          size="small"
          :disabled="blobsPage * blobsPageSize >= blobsData.total"
          @click="onBlobPage(blobsPage + 1)"
        >下一页</n-button>
      </div>
    </n-card>

    <n-card title="校验与修复" size="small" class="mb">
      <n-space vertical :size="12">
        <n-checkbox v-model:checked="verifyApply">应用修复（会重写 ref_count / billed_user_id，按计费关系重算 quota）</n-checkbox>
        <n-checkbox v-model:checked="verifyGC" :disabled="!verifyApply">额外清理磁盘孤儿（仅 apply=true 有效）</n-checkbox>
        <div class="hint">
          校验会统计：<b>blob_mismatch</b>（引用数/计费归属不一致的实体）、
          <b>orphan_blobs</b>（无索引引用却仍登记的实体）、
          <b>orphan_files</b>（磁盘上无索引引用的实体文件）、
          <b>legacy_rows</b>（尚未迁移的存量索引）、
          <b>quota_changed</b>（需要重算配额的用户）。
        </div>
        <n-button type="primary" :loading="verifyLoading" @click="requestVerify">
          <template #icon><n-icon><CheckmarkCircleOutline /></n-icon></template>
          {{ verifyApply ? '执行校验并修复' : '仅预览差异' }}
        </n-button>
        <div v-if="verifyReport" class="report">
          <pre>{{ JSON.stringify(verifyReport, null, 2) }}</pre>
        </div>
      </n-space>
    </n-card>

    <n-modal v-model:show="refsVisible" preset="card" title="实体引用" style="width: 760px" :bordered="false">
      <template #header>
        <div class="refs-header">
          <span>引用</span>
          <code class="hash-text ml">{{ refsHash }}</code>
        </div>
      </template>
      <n-spin :show="refsLoading">
        <table class="data-table">
          <thead>
            <tr>
              <th style="width:80px">文件ID</th>
              <th>文件名</th>
              <th style="width:100px">用户</th>
              <th style="width:100px">大小</th>
              <th style="width:80px">回收站</th>
              <th style="width:160px">创建时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in refsData" :key="r.file_id">
              <td>{{ r.file_id }}</td>
              <td><n-ellipsis style="max-width:260px">{{ r.name }}</n-ellipsis></td>
              <td>{{ r.username }}</td>
              <td>{{ formatSize(r.size) }}</td>
              <td>
                <n-tag v-if="r.is_deleted" size="tiny" type="warning">回收站</n-tag>
                <span v-else class="text-muted">正常</span>
              </td>
              <td>{{ r.created_at }}</td>
            </tr>
            <tr v-if="!refsData.length && !refsLoading">
              <td colspan="6" class="empty-row">暂无引用（该实体可能被误登记或已被回收）</td>
            </tr>
          </tbody>
        </table>
      </n-spin>
    </n-modal>
  </div>
</template>

<style scoped>
.storage-page {
  padding: 16px;
}
.mb { margin-bottom: 16px; }
.stat-num { font-weight: 600; font-variant-numeric: tabular-nums; }
.stat-num.success { color: var(--success-color); }
.hash-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
  color: var(--primary-color);
}
.ml { margin-left: 8px; }
.refs-header {
  display: flex;
  align-items: center;
  font-size: 16px;
  font-weight: 600;
}
.hint {
  font-size: 12px;
  color: var(--n-text-color-2);
  line-height: 1.6;
}
.report pre {
  background: var(--n-card-color-popup);
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  padding: 12px;
  font-size: 12px;
  max-height: 400px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.pagination {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}
.page-info { color: var(--n-text-color-2); font-size: 12px; }
.empty-row { text-align: center; color: var(--n-text-color-3); padding: 24px 0; }
.text-muted { color: var(--n-text-color-3); }
</style>
