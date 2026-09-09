<script setup lang="ts">
import { formatSize } from '@/utils/format'
import type { UploadTaskItem } from '@/utils/uploader'

defineProps<{ visible: boolean; tasks: UploadTaskItem[] }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

function statusText(t: UploadTaskItem): string {
  switch (t.status) {
    case 'hashing':
      return '校验中'
    case 'uploading':
      return `上传中 ${t.progress}%`
    case 'merging':
      return '合并中'
    case 'done':
      return '已完成'
    case 'error':
      return '上传失败'
  }
}
</script>

<template>
  <n-drawer :show="visible" placement="right" :width="380" @update:show="(v: boolean) => emit('update:visible', v)">
    <n-drawer-content title="上传任务" closable>
      <div v-if="tasks.length === 0" class="upload-empty">
        <n-empty description="暂无上传任务" />
      </div>
      <div v-for="t in tasks" :key="t.id" class="upload-item">
        <div class="upload-item-head">
          <span class="upload-name" :title="t.name">{{ t.name }}</span>
          <span class="upload-size">{{ formatSize(t.size) }}</span>
        </div>
        <n-progress
          type="line"
          :percentage="t.progress"
          :height="6"
          :border-radius="3"
          :show-indicator="false"
          :status="t.status === 'error' ? 'error' : t.status === 'done' ? 'success' : 'default'"
        />
        <div class="upload-item-foot">
          <span class="upload-status" :class="{ ok: t.status === 'done', error: t.status === 'error' }">{{ statusText(t) }}</span>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>
