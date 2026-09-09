<script setup lang="ts">
import { ref, watch } from 'vue'
import { searchUsers, sendTransfer } from '@/api/transfer'
import type { FileItem, UserBrief } from '@/api/types'
import FileIcon from './FileIcon.vue'
import { formatSize } from '@/utils/format'
import { message } from '@/utils/notify'

const props = defineProps<{ file: FileItem | null; visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'sent'): void
}>()

const keyword = ref('')
const users = ref<UserBrief[]>([])
const selected = ref<UserBrief | null>(null)
const searching = ref(false)
const sending = ref(false)
let debounceTimer: ReturnType<typeof setTimeout> | null = null

function reset() {
  keyword.value = ''
  users.value = []
  selected.value = null
}

watch(
  () => props.visible,
  (v) => {
    if (v) reset()
  }
)

function onSearch() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(async () => {
    const kw = keyword.value.trim()
    if (!kw) {
      users.value = []
      return
    }
    searching.value = true
    try {
      users.value = await searchUsers(kw)
    } catch {
      /* 拦截器已提示 */
    } finally {
      searching.value = false
    }
  }, 300)
}

function pick(u: UserBrief) {
  selected.value = selected.value?.id === u.id ? null : u
}

function close() {
  emit('update:visible', false)
}

async function onSend() {
  if (!props.file || !selected.value) return
  sending.value = true
  try {
    await sendTransfer({ receiver_id: selected.value.id, file_id: props.file.id })
    message.success(`已发送给 ${selected.value.username}`)
    close()
    emit('sent')
  } catch {
    /* 拦截器已提示 */
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <n-modal :show="visible" preset="card" title="发送文件" style="width: 460px" :bordered="false" @update:show="(v: boolean) => emit('update:visible', v)">
    <div v-if="file">
      <div class="send-file">
        <FileIcon :file="file" :size="28" />
        <div style="min-width: 0">
          <div class="share-file-name">{{ file.name }}</div>
          <div class="share-file-meta">{{ file.type === 0 ? '文件夹' : formatSize(file.size) }}</div>
        </div>
      </div>

      <div class="send-search">
        <n-input v-model:value="keyword" placeholder="输入用户名搜索接收者" clearable @input="onSearch" />
      </div>

      <n-spin :show="searching">
        <div v-if="users.length" class="send-users">
          <div
            v-for="u in users"
            :key="u.id"
            class="send-user"
            :class="{ active: selected?.id === u.id }"
            @click="pick(u)"
          >
            <n-avatar round size="small" :style="{ background: '#2563EB' }">{{ u.username.slice(0, 1)?.toUpperCase() }}</n-avatar>
            <div class="send-user-info">
              <div class="send-user-name">{{ u.username }}</div>
              <div class="send-user-email">{{ u.email || '暂无邮箱' }}</div>
            </div>
            <n-icon v-if="selected?.id === u.id" size="18" color="#2563EB"><svg viewBox="0 0 512 512"><path fill="currentColor" d="M448 256c0-106-86-192-192-192S64 150 64 256s86 192 192 192 192-86 192-192z" /><path fill="none" stroke="currentColor" stroke-width="32" stroke-linecap="round" stroke-linejoin="round" d="M352 176L217.6 336 160 272" /></svg></n-icon>
          </div>
        </div>
        <n-empty v-else-if="!searching && keyword.trim()" description="未找到匹配的用户" style="padding: 16px 0" />
      </n-spin>

      <div v-if="selected" class="send-target">将发送给：{{ selected.username }}</div>

      <div class="modal-footer">
        <n-button @click="close">取消</n-button>
        <n-button type="primary" :loading="sending" :disabled="!selected" @click="onSend">发送</n-button>
      </div>
    </div>
  </n-modal>
</template>
