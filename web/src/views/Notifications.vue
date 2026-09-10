<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowBackOutline, NotificationsOutline } from '@vicons/ionicons5'
import { acceptTransfer, rejectTransfer } from '@/api/transfer'
import { listNotifications, readNotification, unreadCount } from '@/api/notification'
import type { NotificationItem } from '@/api/types'
import { message } from '@/utils/notify'
import { formatDate } from '@/utils/format'

const router = useRouter()

const loading = ref(false)
const items = ref<NotificationItem[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)
const filter = ref<'all' | 'unread' | 'transfer'>('all')
const unread = ref(0)

async function load() {
  loading.value = true
  try {
    const data = await listNotifications(page.value, pageSize)
    let arr = data.items ?? []
    if (filter.value === 'unread') arr = arr.filter((n) => n.is_read === 0)
    if (filter.value === 'transfer') arr = arr.filter((n) => n.type === 'transfer')
    items.value = arr
    total.value = data.total
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function loadUnread() {
  try {
    const u = await unreadCount()
    unread.value = u.count
  } catch {
    /* ignore */
  }
}

async function onMarkRead(n: NotificationItem) {
  if (n.is_read === 1) return
  try {
    await readNotification(n.id)
    n.is_read = 1
    unread.value = Math.max(0, unread.value - 1)
  } catch {
    /* 拦截器已提示 */
  }
}

async function onMarkAllRead() {
  if (!unread.value) return
  try {
    await Promise.all(items.value.filter((n) => n.is_read === 0).map((n) => readNotification(n.id)))
    for (const n of items.value) n.is_read = 1
    unread.value = 0
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
    load()
    loadUnread()
  } catch {
    /* 拦截器已提示 */
  }
}

async function onReject(n: NotificationItem) {
  if (!n.transfer_id) return
  try {
    await rejectTransfer(n.transfer_id)
    message.success('已拒绝')
    load()
    loadUnread()
  } catch {
    /* 拦截器已提示 */
  }
}

function setFilter(f: typeof filter.value) {
  filter.value = f
  page.value = 1
  load()
}

onMounted(() => {
  load()
  loadUnread()
})
</script>

<template>
  <div class="home-page">
    <header class="home-header">
      <div class="home-header-inner">
        <div class="home-brand">
          <n-button quaternary circle size="small" title="返回我的网盘" @click="router.push('/')">
            <template #icon><n-icon><ArrowBackOutline /></n-icon></template>
          </n-button>
          <div class="brand-mark">HC</div>
          <div>
            <div class="brand-name">通知</div>
            <div class="shared-subtitle">系统通知、文件转送请求</div>
          </div>
        </div>
      </div>
    </header>

    <main class="home-main">
      <div class="toolbar">
        <div class="page-title">全部通知 <span v-if="unread" class="notify-badge-inline">{{ unread }} 条未读</span></div>
        <div class="toolbar-right">
          <n-button-group>
            <n-button :type="filter === 'all' ? 'primary' : 'default'" size="small" @click="setFilter('all')">全部</n-button>
            <n-button :type="filter === 'unread' ? 'primary' : 'default'" size="small" @click="setFilter('unread')">未读</n-button>
            <n-button :type="filter === 'transfer' ? 'primary' : 'default'" size="small" @click="setFilter('transfer')">转送</n-button>
          </n-button-group>
          <n-button quaternary size="small" :disabled="unread === 0" @click="onMarkAllRead">全部已读</n-button>
        </div>
      </div>

      <n-spin :show="loading">
        <div v-if="items.length === 0 && !loading" class="notify-empty-page">
          <n-icon :size="48" color="#cbd5e1"><NotificationsOutline /></n-icon>
          <div class="notify-empty-page-text">暂无通知</div>
        </div>

        <div v-else class="notify-page-list">
          <div
            v-for="n in items"
            :key="n.id"
            class="notify-card"
            :class="{ unread: n.is_read === 0 }"
            @click="onMarkRead(n)"
          >
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
        </div>
      </n-spin>
    </main>
  </div>
</template>
