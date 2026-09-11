<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowBackOutline,
  CopyOutline,
  CreateOutline,
  LinkOutline,
  RefreshOutline,
  TrashOutline
} from '@vicons/ionicons5'
import { cancelShare, listMyShares, updateShare } from '@/api/share'
import type { MyShareItem } from '@/api/types'
import { dialog, message } from '@/utils/notify'
import { formatDate, formatSize } from '@/utils/format'

const router = useRouter()

const shares = ref<MyShareItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const data = await listMyShares(1, 200)
    shares.value = data.items ?? []
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

function shareUrl(token: string) {
  return `${location.origin}/share/${token}`
}

async function onCopy(s: MyShareItem) {
  const url = shareUrl(s.token)
  try {
    await navigator.clipboard.writeText(url)
    message.success('分享链接已复制')
  } catch {
    message.warning('复制失败，请手动复制')
  }
}

function onCancel(s: MyShareItem) {
  dialog.warning({
    title: '取消分享',
    content: `取消后“${s.file_name}”的分享链接将立即失效。确定取消吗？`,
    positiveText: '取消分享',
    negativeText: '返回',
    onPositiveClick: async () => {
      try {
        await cancelShare(s.id)
        message.success('已取消分享')
        load()
      } catch {
        /* 拦截器已提示 */
      }
    }
  })
}

/* 编辑弹窗 */
const editVisible = ref(false)
const editTarget = ref<MyShareItem | null>(null)
const editExpire = ref<'forever' | '1d' | '7d' | '30d'>('forever')
const editPassword = ref('')
const editClearPwd = ref(false)

function openEdit(s: MyShareItem) {
  editTarget.value = s
  editExpire.value = s.expire_at ? '7d' : 'forever'
  editPassword.value = ''
  editClearPwd.value = false
  editVisible.value = true
}

function expireDate(opt: string): string | null {
  if (opt === 'forever') return null
  const days = { '1d': 1, '7d': 7, '30d': 30 }[opt] ?? 0
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toISOString()
}

async function onSaveEdit() {
  if (!editTarget.value) return
  let password: string | undefined
  if (editClearPwd.value) password = ''
  else if (editPassword.value) password = editPassword.value
  // M1 修复：选择"永久"时发送 clear_expire=true 清除过期时间（原实现 expire_at 传 null 后端不更新，
  // 导致改为永久后原过期时间仍生效）
  const forever = editExpire.value === 'forever'
  try {
    await updateShare(editTarget.value.id, {
      password,
      expire_at: forever ? undefined : expireDate(editExpire.value),
      clear_expire: forever ? true : undefined
    })
    message.success('分享设置已更新')
    editVisible.value = false
    load()
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
          <div class="brand-mark">HC</div>
          <span class="brand-name">我的分享</span>
        </div>
        <div class="header-right">
          <n-button quaternary @click="router.push('/')">
            <template #icon><n-icon><ArrowBackOutline /></n-icon></template>
            返回我的网盘
          </n-button>
        </div>
      </div>
    </header>

    <main class="home-main">
      <div class="toolbar">
        <div class="toolbar-info">
          <span class="page-title">分享链接管理</span>
          <span class="shared-subtitle">共 {{ shares.length }} 个分享</span>
        </div>
        <div class="toolbar-right">
          <n-button quaternary circle size="small" title="刷新" @click="load">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
          </n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div v-if="shares.length" class="file-list-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>文件</th>
                <th style="width: 90px">大小</th>
                <th style="width: 110px">有效期</th>
                <th style="width: 70px">访问</th>
                <th style="width: 70px">密码</th>
                <th style="width: 150px">创建时间</th>
                <th style="width: 150px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in shares" :key="s.id" :class="{ 'row-expired': s.is_expired }">
                <td>
                  <div class="cell-name">
                    <n-icon :size="18" color="#2563eb"><LinkOutline /></n-icon>
                    <span :title="s.file_name">{{ s.file_name }}</span>
                    <span v-if="s.is_expired" class="share-expired-tag">已过期</span>
                  </div>
                </td>
                <td>{{ s.file_type === 0 ? '-' : formatSize(s.file_size) }}</td>
                <td>{{ s.expire_at ? formatDate(s.expire_at) : '永久' }}</td>
                <td>{{ s.views }}</td>
                <td>{{ s.password_required ? '有' : '无' }}</td>
                <td>{{ formatDate(s.created_at) }}</td>
                <td>
                  <n-space :size="4">
                    <n-button quaternary size="tiny" :disabled="s.is_expired" @click="onCopy(s)">
                      <template #icon><n-icon><CopyOutline /></n-icon></template>
                      复制
                    </n-button>
                    <n-button quaternary size="tiny" @click="openEdit(s)">
                      <template #icon><n-icon><CreateOutline /></n-icon></template>
                      设置
                    </n-button>
                    <n-button quaternary size="tiny" type="error" @click="onCancel(s)">
                      <template #icon><n-icon><TrashOutline /></n-icon></template>
                      取消
                    </n-button>
                  </n-space>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <n-empty v-else-if="!loading" description="还没有分享链接，去我的网盘分享文件吧" style="padding: 60px 0" />
      </n-spin>
    </main>

    <!-- 编辑分享弹窗 -->
    <n-modal v-model:show="editVisible" preset="card" title="分享设置" style="width: 420px" :bordered="false">
      <div class="edit-share-file">
        <n-icon :size="18" color="#2563eb"><LinkOutline /></n-icon>
        <span class="edit-share-name">{{ editTarget?.file_name }}</span>
      </div>
      <n-form label-placement="top">
        <n-form-item label="有效期">
          <n-radio-group v-model:value="editExpire">
            <n-space>
              <n-radio value="forever">永久</n-radio>
              <n-radio value="1d">1 天</n-radio>
              <n-radio value="7d">7 天</n-radio>
              <n-radio value="30d">30 天</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="访问密码">
          <n-input v-model:value="editPassword" type="password" show-password-on="click" placeholder="留空保持不变" />
        </n-form-item>
        <n-checkbox v-model:checked="editClearPwd">清除现有密码</n-checkbox>
      </n-form>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="editVisible = false">取消</n-button>
          <n-button type="primary" @click="onSaveEdit">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
