<script setup lang="ts">
import { SearchOutline, CheckmarkCircleOutline } from '@vicons/ionicons5'
import { computed, ref, watch } from 'vue'
import { createTag, deleteTag, listTags, searchTags, setFileTags, type TagItem } from '@/api/tags'
import { dialog, message } from '@/utils/notify'

const props = defineProps<{ file: { id: number; name: string } | null; visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'updated'): void }>()

const tags = ref<TagItem[]>([])
const selectedIds = ref<number[]>([])
const loading = ref(false)
const saving = ref(false)
const showCreate = ref(false)
const newTagName = ref('')
const newTagColor = ref('#6366f1')
const searching = ref(false)
const searchResult = ref<TagItem[]>([])
const searchKeyword = ref('')
const searchTimer = ref<ReturnType<typeof setTimeout> | null>(null)

const colorPresets = [
  '#6366f1', '#8b5cf6', '#ec4899', '#f43f5e',
  '#f97316', '#eab308', '#22c55e', '#14b8a6',
  '#06b6d4', '#3b82f6', '#64748b', '#1e293b'
]

const filteredTags = computed(() => {
  if (searchResult.value.length) return searchResult.value
  return tags.value
})

async function loadTags() {
  try {
    tags.value = await listTags()
    if (props.file) {
      // 文件已关联的标签通过 Home.vue 传入或使用空数组
      selectedIds.value = []
    }
  } catch {
    /* ignore */
  }
}

watch(() => props.visible, (v) => {
  if (v) {
    loadTags()
    showCreate.value = false
    searchKeyword.value = ''
    searchResult.value = []
  }
})

function toggle(id: number) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = Array.from(next)
}

async function onSave() {
  if (!props.file || !selectedIds.value.length) return
  saving.value = true
  try {
    await setFileTags({ file_id: props.file.id, tag_ids: selectedIds.value })
    message.success('标签已保存')
    emit('updated')
    emit('update:visible', false)
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}

async function onCreate() {
  const name = newTagName.value.trim()
  if (!name) return message.warning('请输入标签名称')
  try {
    await createTag({ name, color: newTagColor.value })
    message.success('标签创建成功')
    newTagName.value = ''
    showCreate.value = false
    loadTags()
  } catch {
    /* 拦截器已提示 */
  }
}

async function onDeleteTag(tag: TagItem) {
  dialog.warning({
    title: '删除标签',
    content: `确定删除标签「${tag.name}」吗？关联的文件不会受影响。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteTag(tag.id)
        message.success('已删除')
        loadTags()
      } catch {
        /* 拦截器已提示 */
      }
    }
  })
}

function onSearchInput() {
  const kw = searchKeyword.value.trim()
  if (searchTimer.value) clearTimeout(searchTimer.value)
  if (!kw) {
    searchResult.value = []
    return
  }
  searchTimer.value = setTimeout(async () => {
    searching.value = true
    try {
      searchResult.value = await searchTags(kw)
    } catch {
      /* ignore */
    } finally {
      searching.value = false
    }
  }, 300)
}

async function onQuickAdd(tag: TagItem) {
  if (!props.file) return
  const next = new Set(selectedIds.value)
  next.add(tag.id)
  selectedIds.value = Array.from(next)
  try {
    await setFileTags({ file_id: props.file.id, tag_ids: selectedIds.value })
    message.success(`已添加标签「${tag.name}」`)
    emit('updated')
  } catch {
    /* ignore */
  }
}
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    title="管理标签"
    style="width: 480px"
    :bordered="false"
    @update:show="(v: boolean) => emit('update:visible', v)"
  >
    <div v-if="file" class="tag-dialog-body">
      <!-- 快速选择区域 -->
      <div class="tag-section">
        <div class="tag-section-head">
          <span>关联标签</span>
          <span v-if="selectedIds.length" class="tag-count">{{ selectedIds.length }} 个已选</span>
        </div>
        <div class="tag-chips">
          <div
            v-for="tag in tags"
            :key="tag.id"
            class="tag-chip"
            :class="{ active: selectedIds.includes(tag.id) }"
            :style="selectedIds.includes(tag.id) ? { borderColor: tag.color, background: tag.color + '18' } : {}"
            @click="toggle(tag.id)"
          >
            <span class="tag-chip-dot" :style="{ background: tag.color }" />
            {{ tag.name }}
          </div>
          <div v-if="!tags.length && !loading" class="tag-empty-hint">
            暂无标签，先创建一个吧
          </div>
        </div>
      </div>

      <!-- 搜索添加 -->
      <div class="tag-section">
        <div class="tag-section-head">搜索标签</div>
        <div class="tag-search-row">
          <n-input
            v-model:value="searchKeyword"
            placeholder="输入标签名称搜索..."
            clearable
            size="small"
            @input="onSearchInput"
          >
            <template #prefix><n-icon :size="14"><SearchOutline /></n-icon></template>
          </n-input>
        </div>
        <div v-if="searching" class="tag-loading">
          <n-spin size="small" />
        </div>
        <div v-else-if="searchResult.length" class="tag-chips">
          <div
            v-for="tag in searchResult"
            :key="tag.id"
            class="tag-chip tag-chip-add"
            :style="{ borderColor: tag.color + '60' }"
            @click="onQuickAdd(tag)"
          >
            <span class="tag-chip-dot" :style="{ background: tag.color }" />
            {{ tag.name }}
            <n-icon :size="13"><CheckmarkCircleOutline v-if="selectedIds.includes(tag.id)" /></n-icon>
          </div>
        </div>
      </div>

      <!-- 创建新标签 -->
      <div class="tag-section">
        <div class="tag-section-head">
          <span>新建标签</span>
          <n-button quaternary size="tiny" @click="showCreate = !showCreate">
            {{ showCreate ? '收起' : '新建' }}
          </n-button>
        </div>
        <div v-if="showCreate" class="tag-create-form">
          <n-input
            v-model:value="newTagName"
            placeholder="标签名称"
            size="small"
            @keyup.enter="onCreate"
          />
          <div class="tag-color-row">
            <div
              v-for="c in colorPresets"
              :key="c"
              class="tag-color-pick"
              :class="{ active: newTagColor === c }"
              :style="{ background: c }"
              @click="newTagColor = c"
            />
          </div>
          <n-button size="small" type="primary" :loading="saving" @click="onCreate">创建</n-button>
        </div>
      </div>

      <!-- 标签管理列表 -->
      <div class="tag-section">
        <div class="tag-section-head">我的标签</div>
        <div class="tag-list">
          <div v-for="tag in tags" :key="tag.id" class="tag-list-item">
            <span class="tag-list-dot" :style="{ background: tag.color }" />
            <span class="tag-list-name">{{ tag.name }}</span>
            <n-button quaternary size="tiny" type="error" @click="onDeleteTag(tag)">删除</n-button>
          </div>
          <div v-if="!tags.length && !loading" class="tag-empty-hint">还没有创建任何标签</div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="modal-footer">
        <n-button @click="emit('update:visible', false)">关闭</n-button>
        <n-button
          v-if="file"
          type="primary"
          :loading="saving"
          :disabled="!selectedIds.length"
          @click="onSave"
        >
          保存
        </n-button>
      </div>
    </template>
  </n-modal>
</template>
