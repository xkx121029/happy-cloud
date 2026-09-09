<script setup lang="ts">
import { computed, h, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  FolderOpenOutline,
  HomeOutline,
  ListOutline,
  LogOutOutline,
  PeopleOutline,
  SpeedometerOutline
} from '@vicons/ionicons5'
import type { MenuOption } from 'naive-ui'
import { NIcon } from 'naive-ui'
import { useUserStore } from '@/stores/user'

const store = useUserStore()
const route = useRoute()
const router = useRouter()

const activeKey = computed(() => route.path)

function renderIcon(icon: Component) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions: MenuOption[] = [
  { label: '系统监控', key: '/admin/monitor', icon: renderIcon(SpeedometerOutline) },
  { label: '用户管理', key: '/admin/users', icon: renderIcon(PeopleOutline) },
  { label: '文件管理', key: '/admin/files', icon: renderIcon(FolderOpenOutline) },
  { label: '操作日志', key: '/admin/logs', icon: renderIcon(ListOutline) }
]

const userMenuOptions = [
  { label: '返回前台', key: 'home', icon: renderIcon(HomeOutline) },
  { label: '退出登录', key: 'logout', icon: renderIcon(LogOutOutline) }
]

const pageTitles: Record<string, string> = {
  '/admin/monitor': '系统监控',
  '/admin/users': '用户管理',
  '/admin/files': '文件管理',
  '/admin/logs': '操作日志'
}
const pageTitle = computed(() => pageTitles[route.path] ?? '管理后台')

function onMenuClick(key: string) {
  router.push(key)
}

function onUserSelect(key: string | number) {
  if (key === 'home') router.push('/')
  if (key === 'logout') {
    store.logout()
    router.replace('/login')
  }
}
</script>

<template>
  <div class="admin-wrap">
    <n-layout has-sider position="absolute" style="height: 100%">
      <n-layout-sider bordered :width="200" content-style="padding: 0">
        <div class="admin-logo">Happy-Cloud <span>管理后台</span></div>
        <n-menu :value="activeKey" :options="menuOptions" @update:value="onMenuClick" />
      </n-layout-sider>
      <n-layout>
        <n-layout-header bordered class="admin-header">
          <div class="admin-title">{{ pageTitle }}</div>
          <n-dropdown :options="userMenuOptions" @select="onUserSelect">
            <div class="admin-user">
              <n-avatar round size="small" :style="{ background: '#2563EB' }">{{ store.user?.username?.slice(0, 1)?.toUpperCase() }}</n-avatar>
              <span>{{ store.user?.username }}</span>
            </div>
          </n-dropdown>
        </n-layout-header>
        <n-layout-content content-style="padding: 16px" :native-scrollbar="false">
          <router-view />
        </n-layout-content>
      </n-layout>
    </n-layout>
  </div>
</template>
