<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getShare, verifyShare } from '@/api/share'
import type { FileItem, ShareInfo } from '@/api/types'
import FileIcon from '@/components/FileIcon.vue'
import { downloadShared } from '@/utils/download'
import { formatDate, formatSize } from '@/utils/format'
import { message } from '@/utils/notify'

const route = useRoute()
const token = String(route.params.token)

const info = ref<ShareInfo | null>(null)
const password = ref('')
const loading = ref(false)
const verifying = ref(false)
// 密码校验通过标记：用于展示文件列表（不依赖 password_required，后者在带密码请求下仍为 true）
const verified = ref(false)
let verifiedPwd = ''

async function load() {
  loading.value = true
  try {
    // 密码分享：验证后需携带密码才能取到文件列表
    info.value = await getShare(token, verifiedPwd)
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function onVerify() {
  if (!password.value.trim()) {
    message.warning('请输入访问密码')
    return
  }
  verifying.value = true
  try {
    await verifyShare(token, password.value.trim())
    verified.value = true
    verifiedPwd = password.value.trim()
    await load()
  } catch {
    /* ignore */
  } finally {
    verifying.value = false
  }
}

function fileList(): FileItem[] {
  if (info.value?.file) return [info.value.file]
  return info.value?.files ?? []
}

async function onDownload(f: FileItem) {
  if (f.type !== 1) return
  try {
    await downloadShared(token, f.id, f.name, verifiedPwd)
  } catch {
    /* 拦截器已提示 */
  }
}

onMounted(load)
</script>

<template>
  <div class="share-page">
    <n-card class="share-card">
      <div class="share-head">
        <div class="brand-mark">HC</div>
        <div>
          <h2>Happy-Cloud 文件分享</h2>
          <p>分享自 Happy-Cloud 云盘</p>
        </div>
      </div>

      <n-spin :show="loading">
        <p v-if="!info && !loading" style="color: #6b7280">分享不存在或已失效。</p>

        <div v-if="info && info.password_required && !verified" class="share-pwd">
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="请输入访问密码" @keyup.enter="onVerify" />
          <n-button type="primary" :loading="verifying" @click="onVerify">验证</n-button>
        </div>

        <div v-if="info && (!info.password_required || verified)">
          <div v-for="f in fileList()" :key="f.id" class="share-item">
            <FileIcon :file="f" :size="26" />
            <div style="flex: 1; min-width: 0">
              <div class="share-item-name" :title="f.name">{{ f.name }}</div>
              <div class="share-item-meta">{{ f.type === 0 ? '文件夹' : `${formatSize(f.size)} · ${formatDate(f.created_at)}` }}</div>
            </div>
            <n-button v-if="f.type === 1" size="small" type="primary" @click="onDownload(f)">下载</n-button>
          </div>
          <n-empty v-if="fileList().length === 0" description="该分享暂无内容" style="padding: 24px 0" />
        </div>
      </n-spin>
    </n-card>
  </div>
</template>
