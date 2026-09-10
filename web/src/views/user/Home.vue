<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  AddCircleOutline,
  CloudUploadOutline,
  CreateOutline,
  DownloadOutline,
  FolderOpenOutline,
  GlobeOutline,
  GridOutline,
  ListOutline,
  LogOutOutline,
  NotificationsOutline,
  RefreshOutline,
  SearchOutline,
  ShareSocialOutline,
  TrashOutline
} from '@vicons/ionicons5'
import { deleteFiles, fetchQuota, listFiles, mkdir, renameFile, toggleShared } from '@/api/files'
import { acceptTransfer, rejectTransfer } from '@/api/transfer'
import { listNotifications, readNotification, unreadCount } from '@/api/notification'
import { toList, type FileItem, type NotificationItem } from '@/api/types'
import FileIcon from '@/components/FileIcon.vue'
import MoveDialog from '@/components/MoveDialog.vue'
import SendDialog from '@/components/SendDialog.vue'
import SettingsDrawer from '@/components/SettingsDrawer.vue'
import ShareDialog from '@/components/ShareDialog.vue'
import { useSettingsStore } from '@/stores/settings'
import { useUserStore } from '@/stores/user'
import { dialog, message } from '@/utils/notify'
import { downloadFile } from '@/utils/download'
import { formatDate, formatSize } from '@/utils/format'
import { uploadFile, type UploadTaskItem } from '@/utils/uploader'

const store = useUserStore()
const router = useRouter()
const settings = useSettingsStore()

/* ---------- 设置抽屉 ---------- */
const settingsVisible = ref(false)

/* ---------- 目录与文件列表 ---------- */
const pathStack = ref<{ id: number; name: string }[]>([])
const files = ref<FileItem[]>([])
const loading = ref(false)

const currentParentId = computed(() => {
  const len = pathStack.value.length
  return len ? pathStack.value[len - 1].id : 0
})

const quota = ref({ max: 0, used: 0 })
const quotaPercent = computed(() => {
  if (!quota.value.max) return 0
  return Math.min(100, Math.round((quota.value.used / quota.value.max) * 100))
})

const viewMode = ref<'grid' | 'list'>(settings.defaultView)
const searchQuery = ref('')

const filteredFiles = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return files.value
  return files.value.filter((f) => f.name.toLowerCase().includes(q))
})

async function loadList() {
  loading.value = true
  try {
    const data = await listFiles(currentParentId.value)
    files.value = toList(data)
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function loadQuota() {
  try {
    const q = await fetchQuota()
    quota.value = { max: q.quota_max ?? 0, used: q.quota_used ?? 0 }
  } catch {
    /* ignore */
  }
}

function enterFolder(f: FileItem) {
  if (f.type !== 0) return
  pathStack.value.push({ id: f.id, name: f.name })
  loadList()
}

function goTo(index: number) {
  pathStack.value = pathStack.value.slice(0, index)
  loadList()
}

/* ---------- 新建文件夹 ---------- */
const mkdirVisible = ref(false)
const mkdirName = ref('')
const mkdirLoading = ref(false)

async function onCreateFolder() {
  const name = mkdirName.value.trim()
  if (!name) {
    message.warning('请输入文件夹名称')
    return
  }
  mkdirLoading.value = true
  try {
    await mkdir({ parent_id: currentParentId.value, name })
    message.success('文件夹创建成功')
    mkdirVisible.value = false
    mkdirName.value = ''
    loadList()
  } catch {
    /* ignore */
  } finally {
    mkdirLoading.value = false
  }
}

/* ---------- 上传 ---------- */
const fileInput = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const uploadTasks = ref<UploadTaskItem[]>([])
let uploadSeq = 0

/** 上传任务转换为文件项，用于复用文件图标与卡片样式 */
function taskFile(t: UploadTaskItem): FileItem {
  return { id: -t.id, name: t.name, type: 1, size: t.size, parent_id: 0, hash: '', created_at: '' }
}

function taskStatusText(t: UploadTaskItem): string {
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

function taskProgressStatus(t: UploadTaskItem): 'default' | 'success' | 'error' {
  if (t.status === 'error') return 'error'
  if (t.status === 'done') return 'success'
  return 'default'
}

function startUploads(fileList: File[]) {
  if (!fileList.length) return
  for (const file of fileList) {
    const task: UploadTaskItem = { id: ++uploadSeq, name: file.name, size: file.size, status: 'hashing', progress: 0 }
    uploadTasks.value.push(task)
    uploadFile(file, currentParentId.value, {
      onStatus: (s) => {
        task.status = s
      },
      onProgress: (p) => {
        task.progress = p
      }
    })
      .then(() => {
        // 保留"已完成"状态短暂显示，再移除并刷新列表，避免界面瞬间无反馈
        task.status = 'done'
        task.progress = 100
        setTimeout(() => {
          uploadTasks.value = uploadTasks.value.filter((x) => x !== task)
          loadList()
          loadQuota()
        }, 800)
      })
      .catch(() => {
        task.status = 'error'
      })
  }
}

function onPickFiles(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) startUploads(Array.from(input.files))
  input.value = ''
}

function onDrop(e: DragEvent) {
  dragging.value = false
  if (e.dataTransfer?.files?.length) startUploads(Array.from(e.dataTransfer.files))
}

/* ---------- 右键菜单 ---------- */
const ctxMenu = ref<{ show: boolean; x: number; y: number; file: FileItem | null }>({ show: false, x: 0, y: 0, file: null })

function showCtx(e: MouseEvent, f: FileItem) {
  e.stopPropagation()
  ctxMenu.value = { show: true, x: e.clientX, y: e.clientY, file: f }
}

function hideCtx() {
  ctxMenu.value.show = false
}

function onOpen(f: FileItem) {
  if (f.type === 0) enterFolder(f)
}

async function onDownload(f: FileItem) {
  if (f.type !== 1) return
  try {
    await downloadFile(f.id, f.name)
  } catch {
    /* 拦截器已提示 */
  }
}

/* ---------- 重命名 ---------- */
const renameVisible = ref(false)
const renameTarget = ref<FileItem | null>(null)
const renameValue = ref('')
const renameLoading = ref(false)

function openRename(f: FileItem) {
  renameTarget.value = f
  renameValue.value = f.name
  renameVisible.value = true
}

async function onRename() {
  const name = renameValue.value.trim()
  if (!renameTarget.value || !name) {
    message.warning('请输入新名称')
    return
  }
  renameLoading.value = true
  try {
    await renameFile({ file_id: renameTarget.value.id, new_name: name })
    message.success('重命名成功')
    renameVisible.value = false
    loadList()
  } catch {
    /* ignore */
  } finally {
    renameLoading.value = false
  }
}

/* ---------- 移动 / 分享 ---------- */
const moveVisible = ref(false)
const moveTarget = ref<FileItem | null>(null)

function openMove(f: FileItem) {
  moveTarget.value = f
  moveVisible.value = true
}

const shareVisible = ref(false)
const shareTarget = ref<FileItem | null>(null)

function openShare(f: FileItem) {
  shareTarget.value = f
  shareVisible.value = true
}

function onDelete(f: FileItem) {
  dialog.warning({
    title: '删除确认',
    content: `确定要删除“${f.name}”吗？该操作不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteFiles([f.id])
        message.success('删除成功')
        loadList()
        loadQuota()
      } catch {
        /* ignore */
      }
    }
  })
}

/* ---------- 用户菜单 ---------- */
const userOptions = [
  ...(store.isAdmin ? [{ label: '管理后台', key: 'admin' }] : []),
  { label: '退出登录', key: 'logout' }
]

function onUserSelect(key: string | number) {
  if (key === 'admin') router.push('/admin')
  if (key === 'logout') {
    store.logout()
    router.replace('/login')
  }
}

function refresh() {
  loadList()
  loadQuota()
}

/* ---------- 共享到公共目录 ---------- */
const sharingId = ref(0)

async function onToggleShared(f: FileItem) {
  sharingId.value = f.id
  try {
    const isShared = (f.is_shared ?? 0) === 1
    await toggleShared({ file_id: f.id, shared: !isShared })
    message.success(isShared ? '已取消公共共享' : '已共享到公共目录')
    loadList()
  } catch {
    /* 拦截器已提示 */
  } finally {
    sharingId.value = 0
  }
}

/* ---------- 发送给用户 ---------- */
const sendVisible = ref(false)
const sendTarget = ref<FileItem | null>(null)

function openSend(f: FileItem) {
  sendTarget.value = f
  sendVisible.value = true
}

/* ---------- 通知 ---------- */
const notifyOpen = ref(false)
const notifications = ref<NotificationItem[]>([])
const unreadCountValue = ref(0)
const notifyLoading = ref(false)
let notifyTimer: ReturnType<typeof setInterval> | null = null

async function loadNotifications() {
  notifyLoading.value = true
  try {
    const data = await listNotifications(1, 10)
    notifications.value = data.items ?? []
    const u = await unreadCount()
    unreadCountValue.value = u.count
  } catch {
    /* 拦截器已提示 */
  } finally {
    notifyLoading.value = false
  }
}

function onNotifyOpen(open: boolean) {
  notifyOpen.value = open
  if (open) loadNotifications()
}

async function onMarkRead(n: NotificationItem) {
  if (n.is_read === 1 || !n.id) return
  try {
    await readNotification(n.id)
    n.is_read = 1
    unreadCountValue.value = Math.max(0, unreadCountValue.value - 1)
  } catch {
    /* 拦截器已提示 */
  }
}

async function onMarkAllRead() {
  if (!unreadCountValue.value) return
  try {
    // 前端先乐观更新，保持交互流畅；后端补一个 read-all 接口可后续再加
    await Promise.all(notifications.value.filter((n) => n.is_read === 0).map((n) => readNotification(n.id)))
    for (const n of notifications.value) n.is_read = 1
    unreadCountValue.value = 0
    message.success('已全部标记为已读')
  } catch {
    /* 拦截器已提示 */
  }
}

async function onAccept(n: NotificationItem) {
  if (!n.transfer_id) return
  try {
    await acceptTransfer(n.transfer_id)
    message.success('已接受，文件已转存到我的网盘')
    loadNotifications()
    loadList()
    loadQuota()
  } catch {
    /* 拦截器已提示 */
  }
}

async function onReject(n: NotificationItem) {
  if (!n.transfer_id) return
  try {
    await rejectTransfer(n.transfer_id)
    message.success('已拒绝该文件')
    loadNotifications()
  } catch {
    /* 拦截器已提示 */
  }
}

onMounted(() => {
  window.addEventListener('click', hideCtx)
  window.addEventListener('contextmenu', hideCtx)
  loadList()
  loadQuota()
  loadNotifications()
  // 轮询未读通知数，用户在线时及时感知新消息
  notifyTimer = setInterval(async () => {
    try {
      const u = await unreadCount()
      unreadCountValue.value = u.count
    } catch {
      /* ignore */
    }
  }, 60000)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', hideCtx)
  window.removeEventListener('contextmenu', hideCtx)
  if (notifyTimer) clearInterval(notifyTimer)
})
</script>

<template>
  <div class="home-page">
    <header class="home-header">
      <div class="home-header-inner">
        <div class="home-brand">
          <div class="brand-mark">HC</div>
          <span class="brand-name">Happy-Cloud</span>
        </div>
        <div class="header-right">
          <div class="quota-block">
            <n-progress type="line" :percentage="quotaPercent" :height="6" :border-radius="3" :show-indicator="false" />
            <span class="quota-text">{{ formatSize(quota.used) }} / {{ formatSize(quota.max) }}</span>
          </div>
          <n-button quaternary @click="router.push('/shared')">
            <template #icon><n-icon><GlobeOutline /></n-icon></template>
            共享目录
          </n-button>
          <n-button @click="fileInput?.click()">
            <template #icon><n-icon><CloudUploadOutline /></n-icon></template>
            上传文件
          </n-button>
          <n-button @click="mkdirVisible = true">
            <template #icon><n-icon><FolderAddOutline /></n-icon></template>
            新建文件夹
          </n-button>
          <n-dropdown :options="userOptions" @select="onUserSelect">
            <div class="user-chip">
              <n-avatar round size="small" :style="{ background: '#2563EB' }">{{ store.user?.username?.slice(0, 1)?.toUpperCase() }}</n-avatar>
              <span class="user-name">{{ store.user?.username }}</span>
            </div>
          </n-dropdown>
          <n-button quaternary circle title="个性化设置" @click="settingsVisible = true">
            <template #icon><n-icon><SettingsOutline /></n-icon></template>
          </n-button>
        </div>
        <!-- 独立通知按钮：绝对定位在 header 右侧，与主按钮群解耦 -->
        <div class="notify-fab" @click="onNotifyOpen(!notifyOpen)" :title="'通知' + (unreadCountValue ? `（${unreadCountValue} 条未读）` : '')">
          <n-badge :value="unreadCountValue" :max="99" :show="unreadCountValue > 0" type="error">
            <div class="notify-fab-icon"><n-icon :size="20"><NotificationsOutline /></n-icon></div>
          </n-badge>
        </div>
      </div>
    </header>

    <n-drawer :show="notifyOpen" placement="right" :width="380" :bordered="false" @update:show="onNotifyOpen">
      <div class="drawer-head">
        <div class="drawer-title">通知</div>
        <div class="drawer-right">
          <n-button quaternary size="small" :disabled="unreadCountValue === 0" @click="onMarkAllRead">全部已读</n-button>
          <n-button quaternary size="small" @click="router.push('/notifications'); notifyOpen = false">全部</n-button>
        </div>
      </div>
      <n-spin :show="notifyLoading">
        <div v-if="notifications.length === 0 && !notifyLoading" class="notify-empty">
          <n-icon :size="36" color="#cbd5e1"><NotificationsOutline /></n-icon>
          <div>暂无新通知</div>
        </div>
        <div v-for="n in notifications" :key="n.id" class="notify-card" :class="{ unread: n.is_read === 0 }" @click="onMarkRead(n)">
          <div class="notify-card-dot" />
          <div class="notify-card-avatar">
            <n-icon :size="18"><NotificationsOutline /></n-icon>
          </div>
          <div class="notify-card-body">
            <div class="notify-card-head">
              <span class="notify-card-title">{{ n.title }}</span>
              <span class="notify-card-time">{{ formatDate(n.created_at) }}</span>
            </div>
            <div class="notify-card-content">{{ n.content }}</div>
            <div class="notify-card-actions">
              <template v-if="n.type === 'transfer' && n.transfer_status === 0">
                <n-button size="tiny" type="primary" @click.stop="onAccept(n)">接受</n-button>
                <n-button size="tiny" @click.stop="onReject(n)">拒绝</n-button>
              </template>
              <span v-else-if="n.type === 'transfer' && n.transfer_status === 1" class="notify-state ok">已接受</span>
              <span v-else-if="n.type === 'transfer' && n.transfer_status === 2" class="notify-state">已拒绝</span>
              <span v-else class="notify-state">已读</span>
            </div>
          </div>
        </div>
      </n-spin>
    </n-drawer>

    <main class="home-main" @dragenter.prevent="dragging = true" @dragover.prevent @dragleave="dragging = false" @drop.prevent="onDrop">
      <div class="toolbar">
        <n-breadcrumb>
          <n-breadcrumb-item v-for="(p, i) in pathStack" :key="p.id" @click="goTo(i)">{{ p.name }}</n-breadcrumb-item>
        </n-breadcrumb>
        <div class="toolbar-right">
          <n-input v-model:value="searchQuery" placeholder="搜索当前目录" clearable style="width: 240px">
            <template #prefix><n-icon><SearchOutline /></n-icon></template>
          </n-input>
          <n-button-group>
            <n-button :type="viewMode === 'grid' ? 'primary' : 'default'" size="small" @click="viewMode = 'grid'">
              <template #icon><n-icon><GridOutline /></n-icon></template>
            </n-button>
            <n-button :type="viewMode === 'list' ? 'primary' : 'default'" size="small" @click="viewMode = 'list'">
              <template #icon><n-icon><ListOutline /></n-icon></template>
            </n-button>
          </n-button-group>
          <n-button quaternary circle size="small" title="刷新" @click="refresh">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
          </n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div v-if="viewMode === 'grid'" class="file-grid">
          <div v-for="t in uploadTasks" :key="t.id" class="file-card upload-task">
            <div class="file-card-icon"><FileIcon :file="taskFile(t)" :size="42" /></div>
            <div class="file-card-name" :title="t.name">{{ t.name }}</div>
            <div class="file-card-meta">
              <span>{{ taskStatusText(t) }}</span>
              <span class="task-size">{{ formatSize(t.size) }}</span>
            </div>
            <div class="upload-task-progress">
              <n-progress type="line" :percentage="t.progress" :height="6" :border-radius="3" :show-indicator="false" :status="taskProgressStatus(t)" />
            </div>
          </div>
          <div v-for="f in filteredFiles" :key="f.id" class="file-card" @dblclick="onOpen(f)" @contextmenu.prevent="showCtx($event, f)">
            <div class="file-card-icon"><FileIcon :file="f" :size="42" /></div>
            <div class="file-card-name" :title="f.name">{{ f.name }}</div>
            <div class="file-card-meta">{{ f.type === 0 ? '文件夹' : formatSize(f.size) }}</div>
            <div class="file-card-actions" @click.stop>
              <n-button quaternary circle size="small" title="下载" :disabled="f.type === 0" @click="onDownload(f)">
                <template #icon><n-icon><DownloadOutline /></n-icon></template>
              </n-button>
              <n-button quaternary circle size="small" title="重命名" @click="openRename(f)">
                <template #icon><n-icon><CreateOutline /></n-icon></template>
              </n-button>
              <n-button quaternary circle size="small" title="移动" @click="openMove(f)">
                <template #icon><n-icon><FolderAddOutline /></n-icon></template>
              </n-button>
              <n-button quaternary circle size="small" title="分享" @click="openShare(f)">
                <template #icon><n-icon><ShareSocialOutline /></n-icon></template>
              </n-button>
              <n-button quaternary circle size="small" title="删除" @click="onDelete(f)">
                <template #icon><n-icon><TrashOutline /></n-icon></template>
              </n-button>
            </div>
          </div>
        </div>

        <div v-else class="file-list-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>名称</th>
                <th style="width: 110px">大小</th>
                <th style="width: 170px">修改时间</th>
                <th style="width: 240px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="t in uploadTasks" :key="t.id" class="upload-task-row">
                <td>
                  <div class="cell-name">
                    <FileIcon :file="taskFile(t)" :size="22" />
                    <span :title="t.name">{{ t.name }}</span>
                  </div>
                </td>
                <td>{{ formatSize(t.size) }}</td>
                <td>
                  <div class="upload-list-progress">
                    <n-progress type="line" :percentage="t.progress" :height="6" :border-radius="3" :show-indicator="false" :status="taskProgressStatus(t)" />
                    <span class="upload-list-status" :class="{ error: t.status === 'error' }">{{ taskStatusText(t) }}</span>
                  </div>
                </td>
                <td></td>
              </tr>
              <tr v-for="f in filteredFiles" :key="f.id" @dblclick="onOpen(f)" @contextmenu.prevent="showCtx($event, f)">
                <td>
                  <div class="cell-name">
                    <FileIcon :file="f" :size="22" />
                    <span :title="f.name">{{ f.name }}</span>
                  </div>
                </td>
                <td>{{ f.type === 0 ? '-' : formatSize(f.size) }}</td>
                <td>{{ formatDate(f.created_at) }}</td>
                <td>
                  <n-space :size="4">
                    <n-button quaternary size="tiny" :disabled="f.type === 0" @click="onDownload(f)">下载</n-button>
                    <n-button quaternary size="tiny" @click="openRename(f)">重命名</n-button>
                    <n-button quaternary size="tiny" @click="openMove(f)">移动</n-button>
                    <n-button quaternary size="tiny" @click="openShare(f)">分享</n-button>
                    <n-button quaternary size="tiny" type="error" @click="onDelete(f)">删除</n-button>
                  </n-space>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <n-empty v-if="!loading && filteredFiles.length === 0 && uploadTasks.length === 0" description="暂无文件，可拖拽文件到此处上传" style="padding: 60px 0" />
      </n-spin>

      <div v-if="dragging" class="drop-mask">
        <div class="drop-tip">
          <n-icon :size="40"><CloudUploadOutline /></n-icon>
          <p>释放鼠标上传文件</p>
        </div>
      </div>
    </main>

    <input ref="fileInput" type="file" multiple hidden @change="onPickFiles" />

    <div v-if="ctxMenu.show && ctxMenu.file" class="ctx-menu" :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }" @click.stop>
      <div class="ctx-item" @click="onDownload(ctxMenu.file!)">下载</div>
      <div class="ctx-item" @click="openRename(ctxMenu.file!)">重命名</div>
      <div class="ctx-item" @click="openMove(ctxMenu.file!)">移动到</div>
      <div class="ctx-item" @click="openShare(ctxMenu.file!)">分享</div>
      <div v-if="ctxMenu.file!.type === 1" class="ctx-item" @click="onToggleShared(ctxMenu.file!)">
        {{ (ctxMenu.file!.is_shared ?? 0) === 1 ? '取消公共共享' : '共享到公共目录' }}
      </div>
      <div v-if="ctxMenu.file!.type === 1" class="ctx-item" @click="openSend(ctxMenu.file!)">发送给用户</div>
      <div class="ctx-divider" />
      <div class="ctx-item danger" @click="onDelete(ctxMenu.file!)">删除</div>
    </div>

    <n-modal v-model:show="mkdirVisible" preset="card" title="新建文件夹" style="width: 400px" :bordered="false">
      <n-input v-model:value="mkdirName" placeholder="请输入文件夹名称" @keyup.enter="onCreateFolder" />
      <template #footer>
        <div class="modal-footer">
          <n-button @click="mkdirVisible = false">取消</n-button>
          <n-button type="primary" :loading="mkdirLoading" @click="onCreateFolder">创建</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="renameVisible" preset="card" title="重命名" style="width: 400px" :bordered="false">
      <n-input v-model:value="renameValue" @keyup.enter="onRename" />
      <template #footer>
        <div class="modal-footer">
          <n-button @click="renameVisible = false">取消</n-button>
          <n-button type="primary" :loading="renameLoading" @click="onRename">确定</n-button>
        </div>
      </template>
    </n-modal>

    <ShareDialog :visible="shareVisible" :file="shareTarget" @update:visible="shareVisible = $event" />
    <MoveDialog :visible="moveVisible" :file="moveTarget" @update:visible="moveVisible = $event" @moved="loadList" />
    <SendDialog :visible="sendVisible" :file="sendTarget" @update:visible="sendVisible = $event" />
    <SettingsDrawer :visible="settingsVisible" @update:visible="settingsVisible = $event" />
  </div>
</template>
