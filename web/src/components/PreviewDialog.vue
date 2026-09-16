<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  CloudDownloadOutline,
  DocumentTextOutline,
  ImageOutline,
  MusicalNotesOutline,
  PlayCircleOutline,
  DocumentOutline
} from '@vicons/ionicons5'
import type { FileItem } from '@/api/types'
import { useUserStore } from '@/stores/user'
import { downloadFile } from '@/utils/download'
import { formatSize } from '@/utils/format'

const props = defineProps<{ visible: boolean; file: FileItem | null }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const userStore = useUserStore()

type Kind = '' | 'image' | 'vector' | 'video' | 'audio' | 'pdf' | 'text' | 'unsupported'

const loading = ref(false)
const error = ref('')
const blobUrl = ref('')
const streamSrc = ref('')
const textContent = ref('')
const textTruncated = ref(false)
const loadedType = ref<Kind>('')

/** 文本预览最多读取的字节数（后面的内容按 Range 截断，避免超大日志把内存打满） */
const TEXT_PREVIEW_LIMIT = 512 * 1024

const IMAGE_EXTS = ['png', 'jpg', 'jpeg', 'jfif', 'gif', 'webp', 'avif', 'bmp', 'ico', 'apng']
/** 矢量图走 blob：后端为防存储型 XSS 未以内联方式返回 svg */
const VECTOR_EXTS = ['svg', 'svgz']
const VIDEO_EXTS = ['mp4', 'm4v', 'webm', 'ogv', 'ogg', 'mov', 'mkv', '3gp']
const AUDIO_EXTS = ['mp3', 'wav', 'flac', 'm4a', 'aac', 'opus', 'oga', 'weba', 'aif', 'aiff']
const TEXT_EXTS = [
  'txt', 'md', 'markdown', 'log', 'json', 'jsonl', 'ndjson', 'jsonc', 'geojson',
  'yaml', 'yml', 'toml', 'ini', 'conf', 'cfg', 'properties', 'env', 'editorconfig',
  'csv', 'tsv', 'xml', 'plist',
  'css', 'scss', 'less', 'sass', 'styl',
  'js', 'mjs', 'cjs', 'jsx', 'ts', 'tsx', 'vue', 'svelte', 'astro',
  'go', 'py', 'rb', 'php', 'java', 'kt', 'kts', 'scala', 'groovy',
  'rs', 'c', 'h', 'cpp', 'cc', 'cxx', 'hpp', 'cs', 'swift', 'm', 'mm',
  'dart', 'lua', 'r', 'pl', 'pm', 'ex', 'exs', 'erl', 'hs', 'clj',
  'sh', 'bash', 'zsh', 'fish', 'bat', 'cmd', 'ps1',
  'sql', 'graphql', 'gql', 'proto', 'thrift',
  'diff', 'patch', 'srt', 'vtt', 'ass', 'ssa',
  'tex', 'rst', 'adoc', 'org', 'textile',
  'gradle', 'cmake', 'mk', 'lock', 'pem', 'crt', 'key', 'pub',
  'html', 'htm'
]
/** 无扩展名但属于纯文本的常见文件名 */
const TEXT_FILENAMES = [
  'dockerfile', 'makefile', 'license', 'readme', 'changelog',
  'gitignore', 'gitattributes', 'editorconfig', 'npmrc', 'env'
]

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

const previewKind = computed<{ type: Kind; icon: any; label: string }>(() => {
  const name = props.file?.name ?? ''
  const ext = extOf(name)
  const base = name.toLowerCase()
  if (IMAGE_EXTS.includes(ext)) return { type: 'image', icon: ImageOutline, label: '图片预览' }
  if (VECTOR_EXTS.includes(ext)) return { type: 'vector', icon: ImageOutline, label: '图片预览' }
  if (VIDEO_EXTS.includes(ext)) return { type: 'video', icon: PlayCircleOutline, label: '视频预览' }
  if (AUDIO_EXTS.includes(ext)) return { type: 'audio', icon: MusicalNotesOutline, label: '音频预览' }
  if (ext === 'pdf') return { type: 'pdf', icon: DocumentTextOutline, label: 'PDF 预览' }
  if (TEXT_EXTS.includes(ext) || TEXT_FILENAMES.includes(base))
    return { type: 'text', icon: DocumentTextOutline, label: '文本预览' }
  return { type: 'unsupported', icon: DocumentOutline, label: '无法预览' }
})

/** 媒体类型直接引用流地址：由浏览器按 Range 边下边播，不再整文件读入内存，因此不限大小 */
const STREAM_KINDS: Kind[] = ['image', 'video', 'audio', 'pdf']

function streamUrl(id: number): string {
  return `/api/files/preview?file_id=${id}&token=${encodeURIComponent(userStore.token)}`
}

watch(
  () => [props.visible, props.file?.id] as const,
  async ([visible, fid]) => {
    // M7 修复：弹窗关闭时也调用 reset() 释放 blob URL，避免每次打开-关闭泄漏内存
    if (!visible) {
      reset()
      return
    }
    if (!fid) return
    reset()
    await loadPreview()
  }
)

function reset() {
  error.value = ''
  textContent.value = ''
  textTruncated.value = false
  streamSrc.value = ''
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = ''
  }
  loadedType.value = ''
}

async function loadPreview() {
  const f = props.file
  if (!f) return
  const kind = previewKind.value.type
  if (kind === 'unsupported') return

  // 媒体类：交给浏览器流式加载（支持拖动进度条，不受文件大小限制）
  if (STREAM_KINDS.includes(kind)) {
    streamSrc.value = streamUrl(f.id)
    loadedType.value = kind
    return
  }

  loading.value = true
  try {
    if (kind === 'text') {
      const res = await fetch(streamUrl(f.id), {
        headers: { Range: `bytes=0-${TEXT_PREVIEW_LIMIT - 1}` }
      })
      if (!res.ok && res.status !== 206) throw new Error('preview failed')
      // Content-Range 形如 "bytes 0-524287/8388608"，用于判断是否被截断
      const range = res.headers.get('Content-Range')
      const total = range ? Number(range.split('/')[1]) : 0
      textTruncated.value = !!range && total > TEXT_PREVIEW_LIMIT
      textContent.value = await res.text()
      loadedType.value = 'text'
    } else {
      // 矢量图走 blob，避免后端以 octet-stream 返回时浏览器拒绝按图片渲染
      const res = await fetch(streamUrl(f.id))
      if (!res.ok) throw new Error('preview failed')
      blobUrl.value = URL.createObjectURL(await res.blob())
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
        <p class="preview-unsupported-tip">已支持：图片、视频、音频、PDF、以及常见代码 / 配置 / 字幕等文本格式</p>
      </div>

      <div v-else-if="loadedType === 'image' || loadedType === 'vector'" class="preview-image">
        <img :src="loadedType === 'vector' ? blobUrl : streamSrc" :alt="file?.name" />
      </div>

      <div v-else-if="loadedType === 'video'" class="preview-video">
        <video :src="streamSrc" controls autoplay preload="metadata" class="preview-video-el"></video>
      </div>

      <div v-else-if="loadedType === 'audio'" class="preview-audio">
        <div class="preview-audio-icon"><n-icon :size="56"><MusicalNotesOutline /></n-icon></div>
        <audio :src="streamSrc" controls preload="metadata" class="preview-audio-el"></audio>
      </div>

      <div v-else-if="loadedType === 'pdf'" class="preview-pdf">
        <iframe :src="streamSrc" class="preview-pdf-frame" title="PDF 预览"></iframe>
      </div>

      <div v-else-if="loadedType === 'text'" class="preview-text">
        <pre>{{ textContent }}</pre>
        <div v-if="textTruncated" class="preview-text-tip">
          文件较大，仅显示前 {{ formatSize(TEXT_PREVIEW_LIMIT) }}，完整内容请下载查看
        </div>
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