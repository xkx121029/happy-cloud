<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createShare } from '@/api/share'
import type { FileItem } from '@/api/types'
import { formatSize } from '@/utils/format'
import { message } from '@/utils/notify'
import FileIcon from './FileIcon.vue'

const props = defineProps<{ file: FileItem | null; visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const password = ref('')
const expire = ref('0')
const loading = ref(false)
const result = ref<{ token: string; url: string } | null>(null)

const expireOptions = [
  { label: '永久有效', value: '0' },
  { label: '1 天', value: '1' },
  { label: '7 天', value: '7' },
  { label: '30 天', value: '30' }
]

const shareUrl = computed(() => {
  if (!result.value) return ''
  const u = result.value.url
  if (u.startsWith('http')) return u
  return `${window.location.origin}${u.startsWith('/') ? '' : '/'}${u}`
})

function reset() {
  password.value = ''
  expire.value = '0'
  result.value = null
}

watch(
  () => props.visible,
  (v) => {
    if (v) reset()
  }
)

function close() {
  emit('update:visible', false)
}

function fmtExpire(days: number): string {
  const d = new Date(Date.now() + days * 24 * 3600 * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

async function onCreate() {
  if (!props.file) return
  loading.value = true
  try {
    const days = Number(expire.value)
    const data: { file_id: number; password?: string; expire_at?: string } = { file_id: props.file.id }
    const pwd = password.value.trim()
    if (pwd) data.password = pwd
    if (days > 0) data.expire_at = fmtExpire(days)
    const res = await createShare(data)
    result.value = { token: res.token, url: res.url }
    message.success('分享链接创建成功')
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function onCopy() {
  try {
    await navigator.clipboard.writeText(shareUrl.value)
    message.success('链接已复制到剪贴板')
  } catch {
    message.warning('复制失败，请手动复制')
  }
}

function onPreview() {
  window.open(shareUrl.value, '_blank')
}
</script>

<template>
  <n-modal :show="visible" preset="card" title="分享文件" style="width: 460px" :bordered="false" @update:show="(v: boolean) => emit('update:visible', v)">
    <div v-if="file">
      <div class="share-file">
        <FileIcon :file="file" :size="28" />
        <div style="min-width: 0">
          <div class="share-file-name">{{ file.name }}</div>
          <div class="share-file-meta">{{ file.type === 0 ? '文件夹' : formatSize(file.size) }}</div>
        </div>
      </div>

      <div v-if="!result" class="share-form">
        <n-form label-placement="left" label-width="70">
          <n-form-item label="访问密码">
            <n-input v-model:value="password" placeholder="留空则无需密码" clearable />
          </n-form-item>
          <n-form-item label="有效期">
            <n-select v-model:value="expire" :options="expireOptions" />
          </n-form-item>
        </n-form>
        <div class="modal-footer">
          <n-button @click="close">取消</n-button>
          <n-button type="primary" :loading="loading" @click="onCreate">创建分享</n-button>
        </div>
      </div>

      <div v-else>
        <n-input :value="shareUrl" readonly>
          <template #suffix>
            <n-button quaternary size="small" @click="onCopy">复制</n-button>
          </template>
        </n-input>
        <div class="share-result-actions">
          <n-button @click="onPreview">预览分享</n-button>
          <n-button type="primary" @click="close">完成</n-button>
        </div>
      </div>
    </div>
  </n-modal>
</template>
