<script setup lang="ts">
import { ref, watch } from 'vue'
import { ChevronForwardOutline, FolderOutline } from '@vicons/ionicons5'
import { batchMove, listFiles, moveFile } from '@/api/files'
import { toList, type FileItem } from '@/api/types'
import { message } from '@/utils/notify'

// file：单文件移动；fileIds：批量移动（传入时优先使用批量接口）
const props = defineProps<{ file: FileItem | null; visible: boolean; fileIds?: number[] }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'moved'): void
}>()

const stack = ref<{ id: number; name: string }[]>([])
const folders = ref<FileItem[]>([])
const loading = ref(false)
const moving = ref(false)
const isBatch = () => !!props.fileIds && props.fileIds.length > 0

function currentId(): number {
  return stack.value.length ? stack.value[stack.value.length - 1].id : 0
}

async function loadFolders() {
  loading.value = true
  try {
    const data = await listFiles(currentId())
    // L10 修复：批量移动时排除"本次选中的文件夹自身"，避免允许将文件夹移动到自身
    const excludeIds = new Set(props.fileIds ?? [])
    if (props.file) excludeIds.add(props.file.id)
    folders.value = toList(data).filter((f) => f.type === 0 && !excludeIds.has(f.id))
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      stack.value = []
      loadFolders()
    }
  }
)

function goTo(index: number) {
  stack.value = stack.value.slice(0, index)
  loadFolders()
}

function enter(f: FileItem) {
  stack.value.push({ id: f.id, name: f.name })
  loadFolders()
}

function close() {
  emit('update:visible', false)
}

async function onMove() {
  const target = currentId()
  if (isBatch()) {
    if (props.fileIds?.includes(target)) {
      message.warning('不能移动到目标文件夹自身')
      return
    }
    moving.value = true
    try {
      await batchMove({ file_ids: props.fileIds!, target_parent_id: target })
      message.success('移动成功')
      close()
      emit('moved')
    } catch {
      /* 拦截器已提示 */
    } finally {
      moving.value = false
    }
    return
  }
  if (!props.file) return
  if (props.file.type === 0 && target === props.file.id) {
    message.warning('不能将文件夹移动到自身')
    return
  }
  moving.value = true
  try {
    await moveFile({ file_id: props.file.id, target_parent_id: target })
    message.success('移动成功')
    close()
    emit('moved')
  } catch {
    /* 拦截器已提示 */
  } finally {
    moving.value = false
  }
}
</script>

<template>
  <n-modal :show="visible" preset="card" :title="isBatch() ? '批量移动' : '移动文件'" style="width: 520px" :bordered="false" @update:show="(v: boolean) => emit('update:visible', v)">
    <div v-if="file || isBatch()">
      <div class="move-location">
        <span class="move-label">移动到：</span>
        <span class="move-path">
          <span class="path-item" @click="goTo(0)">根目录</span>
          <template v-for="(p, i) in stack" :key="p.id">
            <span class="path-sep"><n-icon :size="12"><ChevronForwardOutline /></n-icon></span>
            <span class="path-item" @click="goTo(i + 1)">{{ p.name }}</span>
          </template>
        </span>
      </div>

      <n-spin :show="loading">
        <div class="move-folders">
          <div v-for="f in folders" :key="f.id" class="move-folder" @dblclick="enter(f)">
            <n-icon :size="20" color="#2563EB"><FolderOutline /></n-icon>
            <span class="move-folder-name" :title="f.name">{{ f.name }}</span>
            <n-button size="tiny" quaternary @click="enter(f)">进入</n-button>
          </div>
          <n-empty v-if="!loading && folders.length === 0" description="当前目录没有子文件夹" style="padding: 24px 0" />
        </div>
      </n-spin>
    </div>

    <template #footer>
      <div v-if="file" class="modal-footer">
        <n-button @click="close">取消</n-button>
        <n-button type="primary" :loading="moving" @click="onMove">移动到此处</n-button>
      </div>
    </template>
  </n-modal>
</template>
