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
// 文件夹分享浏览：面包屑路径 + 当前文件夹
const pathStack = ref<{ id: number; name: string }[]>([])
const currentFolderId = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    // 密码分享：验证后需携带密码才能取到文件列表
    info.value = await getShare(token, verifiedPwd, currentFolderId.value ?? undefined)
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

function enterFolder(f: FileItem) {
  pathStack.value.push({ id: f.id, name: f.name })
  currentFolderId.value = f.id
  load()
}

function goTo(index: number) {
  if (index < 0) {
    // 根目录
    pathStack.value = []
    currentFolderId.value = null
  } else {
    pathStack.value = pathStack.value.slice(0, index + 1)
    const target = pathStack.value[index]
    currentFolderId.value = target?.id ?? null
  }
  load()
}

async function onDownload(f: FileItem) {
  try {
    const isDir = f.type === 0
    // 文件夹：file_id 指向该子文件夹，后端服务端打包 zip；文件：单文件直出
    await downloadShared(token, f.id, isDir ? `${f.name}.zip` : f.name, verifiedPwd)
  } catch {
    /* 拦截器已提示 */
  }
}

async function onDownloadCurrent() {
  if (!info.value?.file) return
  try {
    // 不传 file_id → 后端打包整个（当前）文件夹
    await downloadShared(token, null, `${info.value.file.name}.zip`, verifiedPwd)
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
          <!-- 单文件分享 -->
          <template v-if="info.file && info.file.type === 1">
            <div class="share-item">
              <FileIcon :file="info.file" :size="26" />
              <div style="flex: 1; min-width: 0">
                <div class="share-item-name" :title="info.file.name">{{ info.file.name }}</div>
                <div class="share-item-meta">{{ formatSize(info.file.size) }} · {{ formatDate(info.file.created_at) }}</div>
              </div>
              <n-button size="small" type="primary" @click="onDownload(info.file)">下载</n-button>
            </div>
          </template>

          <!-- 文件夹分享：面包屑 + 子内容浏览 -->
          <template v-else-if="info.file && info.file.type === 0">
            <div class="share-toolbar">
              <div class="share-crumbs">
                <span v-if="!pathStack.length" class="crumb active">{{ info.file.name }}</span>
                <template v-else>
                  <span class="crumb" @click="goTo(-1)">{{ info.file.name }}</span>
                  <template v-for="(p, i) in pathStack" :key="i">
                    <span class="crumb-sep">/</span>
                    <span class="crumb" :class="{ active: i === pathStack.length - 1 }" @click="goTo(i)">{{ p.name }}</span>
                  </template>
                </template>
              </div>
              <n-button size="small" type="primary" ghost @click="onDownloadCurrent">
                <template #icon>
                  <n-icon><svg viewBox="0 0 512 512" width="16" height="16"><path d="M382.56 233.376c-8.544-10.4-23.776-11.936-34.208-3.36L288 281.28V120a24 24 0 0 0-48 0v161.28l-60.352-51.264c-10.432-8.576-25.664-7.04-34.208 3.36-8.512 10.368-7.136 25.664 3.264 34.176l104 85.344a24 24 0 0 0 30.624 0l104-85.344c10.4-8.512 11.776-23.808 3.264-34.176zM112 352v96h288v-96a16 16 0 0 1 32 0v96a48.05 48.05 0 0 1-48 48H128a48.05 48.05 0 0 1-48-48v-96a16 16 0 0 1 32 0z" fill="currentColor"/></svg></n-icon>
                </template>
                下载文件夹
              </n-button>
            </div>

            <div v-if="info.children && info.children.length">
              <div v-for="f in info.children" :key="f.id" class="share-item">
                <FileIcon :file="f" :size="26" />
                <div style="flex: 1; min-width: 0; cursor: pointer" :class="{ 'is-folder': f.type === 0 }" @click="f.type === 0 && enterFolder(f)">
                  <div class="share-item-name" :title="f.name">{{ f.name }}</div>
                  <div class="share-item-meta">{{ f.type === 0 ? '文件夹' : `${formatSize(f.size)} · ${formatDate(f.created_at)}` }}</div>
                </div>
                <n-button v-if="f.type === 0" size="small" @click="enterFolder(f)">打开</n-button>
                <n-button size="small" type="primary" @click="onDownload(f)">下载</n-button>
              </div>
            </div>
            <n-empty v-else description="该文件夹暂无内容" style="padding: 24px 0" />
          </template>

          <n-empty v-if="!info.file" description="该分享暂无内容" style="padding: 24px 0" />
        </div>
      </n-spin>
    </n-card>
  </div>
</template>

<style scoped>
.share-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--hc-border);
  flex-wrap: wrap;
}
.share-crumbs {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  overflow: hidden;
}
.crumb {
  font-size: 14px;
  color: var(--hc-text-secondary);
  cursor: pointer;
  white-space: nowrap;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.crumb:hover {
  color: var(--hc-primary);
}
.crumb.active {
  color: var(--hc-text);
  font-weight: 600;
  cursor: default;
}
.crumb-sep {
  color: var(--hc-text-muted);
  font-size: 12px;
}
.is-folder:hover .share-item-name {
  color: var(--hc-primary);
}
</style>
