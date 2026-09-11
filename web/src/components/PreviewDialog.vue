<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CloudDownloadOutline, DocumentTextOutline, ImageOutline, MusicalNotesOutline, PlayCircleOutline, DocumentOutline } from '@vicons/ionicons5'
import { fetchPreviewBlob } from '@/api/files'
import type { FileItem } from '@/api/types'
import { downloadFile } from '@/utils/download'
import { formatSize } from '@/utils/format'
import { message } from '@/utils/notify'

const props = defineProps<{ visible: boolean; file: FileItem | null }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const loading = ref(false)
const error = ref('')
const blobUrl = ref('')
const textContent = ref('')
const loadedType = ref<'image' | 'video' | 'audio' | 'pdf' | 'text' | 'unsupported' | ''>('')

const MAX_PREVIEW = 100 * 1024 * 1024 // 100MB 以内允许预览

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

const previewKind = computed<{ type: '' | 'image' | 'video' | 'audio' | 'pdf' | 'text' | 'unsupported'; icon: any; label: string }>(() => {
  const ext = extOf(props.file?.name ?? '')
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'].includes(ext))
    return { type: 'image', icon: ImageOutline, label: '图片预览' }
  if (['mp4', 'webm', 'ogg', 'mov', 'm4v'].includes(ext))
    return { type: 'video', icon: PlayCircleOutline, label: '视频预览' }
  if (['mp3', 'wav', 'flac', 'm4a', 'aac'].includes(ext))
    return { type: 'audio', icon: MusicalNotesOutline, label: '音频预览' }
  if (ext === 'pdf') return { type: 'pdf', icon: DocumentTextOutline, label: 'PDF 预览' }
  if (['txt', 'md', 'log', 'json', 'yaml', 'yml', 'csv', 'html', 'htm', 'xml', 'js', 'ts', 'css', 'go', 'py', 'java', 'c', 'cpp'].includes(ext))
    return { type: 'text', icon: DocumentTextOutline, label: '文本预览' }
  return { type: 'unsupported', icon: DocumentOutline, label: '无法预览' }
})

watch(
  () => [props.visible, props.file?.id] as const,
  async ([visible, fid]) => {
    if (!visible || !fid) return
    reset()
    await loadPreview()
  }
)

function reset() {
  error.value = ''
  textContent.value = ''
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = ''
  }
  loadedType.value = ''
}

async function loadPreview() {
  const f = props.file
  if (!f) return
  if (f.size > MAX_PREVIEW) {
    error.value = '文件较大（超过 100MB），暂不支持在线预览，请下载后查看'
    return
  }
  const kind = previewKind.value.type
  if (kind === 'unsupported') return
  loading.value = true
  try {
    const blob = await fetchPreviewBlob(f.id)
    if (kind === 'text') {
      textContent.value = await blob.text()
      loadedType.value = 'text'
    } else {
      blobUrl.value = URL.createObjectURL(blob)
      loadedType.value = kind
    }
  } catch {
    error.value = '预览加载失败'
  } finally {
    loading.value = false
  }
}

function onClose() {
  emit('update:visible', false)
}

async function onDownload() {
  if (!props.file) return
  try {
    await downloadFile(props.file.id, props.file.name)
  } catch {
    /* 拦截器已提示 */
  }
}
</script>

<template>
  <n-modal :show="visible" preset="card" :style="{ width: 'min(880px, 94vw)' }" :bordered="false" :closable="true" @update:show="onClose">
    <template #header>
      <div class="preview-head">
        <n-icon :size="20" :component="previewKind.icon" />
        <span class="preview-title">{{ previewKind.label }}</span>
        <span class="preview-filename">{{ file?.name }}</span>
        <span class="preview-meta">{{ formatSize(file?.size ?? 0) }}</span>
      </div>
    </template>

    <n-spin :show="loading">
      <div v-if="error" class="preview-error">
        <n-icon :size="40" color="#9ca3af"><DocumentOutline /></n-icon>
        <p>{{ error }}</p>
      </div>

      <div v-else-if="previewKind.type === 'unsupported'" class="preview-unsupported">
        <n-icon :size="48" color="#9ca3af"><DocumentOutline /></n-icon>
        <p>暂不支持预览该类型文件，请下载后查看</p>
      </div>

      <div v-else-if="loadedType === 'image'" class="preview-image">
        <img :src="blobUrl" :alt="file?.name" />
      </div>

      <div v-else-if="loadedType === 'video'" class="preview-video">
        <video :src="blobUrl" controls autoplay class="preview-video-el"></video>
      </div>

      <div v-else-if="loadedType === 'audio'" class="preview-audio">
        <div class="preview-audio-icon"><n-icon :size="56"><MusicalNotesOutline /></n-icon></div>
        <audio :src="blobUrl" controls class="preview-audio-el"></audio>
      </div>

      <div v-else-if="loadedType === 'pdf'" class="preview-pdf">
        <iframe :src="blobUrl" class="preview-pdf-frame" title="PDF 预览"></iframe>
      </div>

      <div v-else-if="loadedType === 'text'" class="preview-text">
        <pre>{{ textContent }}</pre>
      </div>
    </n-spin>

    <template #footer>
      <div class="modal-footer">
        <n-button type="primary" :disabled="file?.type !== 1" @click="onDownload">
          <template #icon><n-icon><CloudDownloadOutline /></n-icon></template>
          下载
        </n-button>
      </div>
    </template>
  </n-modal>
</template>
