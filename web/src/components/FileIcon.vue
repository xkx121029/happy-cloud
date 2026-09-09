<script setup lang="ts">
import { computed } from 'vue'
import {
  ArchiveOutline,
  CodeSlashOutline,
  DocumentTextOutline,
  FilmOutline,
  FolderOutline,
  ImageOutline,
  MusicalNotesOutline
} from '@vicons/ionicons5'
import type { FileItem } from '@/api/types'

const props = defineProps<{ file: FileItem; size?: number }>()

const IMAGE_EXT = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico']
const VIDEO_EXT = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v']
const AUDIO_EXT = ['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a', 'wma']
const ARCHIVE_EXT = ['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz']
const CODE_EXT = ['js', 'ts', 'vue', 'jsx', 'tsx', 'html', 'css', 'scss', 'json', 'go', 'java', 'py', 'c', 'cpp', 'h', 'sql', 'sh', 'yml', 'yaml', 'xml']

const ext = computed(() => props.file.name.split('.').pop()?.toLowerCase() ?? '')

const icon = computed(() => {
  if (props.file.type === 0) return FolderOutline
  if (IMAGE_EXT.includes(ext.value)) return ImageOutline
  if (VIDEO_EXT.includes(ext.value)) return FilmOutline
  if (AUDIO_EXT.includes(ext.value)) return MusicalNotesOutline
  if (ARCHIVE_EXT.includes(ext.value)) return ArchiveOutline
  if (CODE_EXT.includes(ext.value)) return CodeSlashOutline
  return DocumentTextOutline
})

const color = computed(() => {
  if (props.file.type === 0) return '#2563EB'
  if (IMAGE_EXT.includes(ext.value)) return '#10B981'
  if (VIDEO_EXT.includes(ext.value)) return '#8B5CF6'
  if (AUDIO_EXT.includes(ext.value)) return '#F59E0B'
  if (ARCHIVE_EXT.includes(ext.value)) return '#EF4444'
  return '#64748B'
})
</script>

<template>
  <n-icon :size="size ?? 32" :color="color" :component="icon" />
</template>
