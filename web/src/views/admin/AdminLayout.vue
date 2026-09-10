<script setup lang="ts">
import { computed, h, ref, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  FolderOpenOutline,
  HomeOutline,
  ListOutline,
  LockClosedOutline,
  LogOutOutline,
  PeopleOutline,
  SpeedometerOutline
} from '@vicons/ionicons5'
import type { MenuOption } from 'naive-ui'
import { NIcon } from 'naive-ui'
import { useUserStore } from '@/stores/user'
import { changeMyPassword } from '@/api/account'
import { message } from '@/utils/notify'

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
  { label: '修改密码', key: 'change-password', icon: renderIcon(LockClosedOutline) },
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

/* ---------- 修改密码弹窗 ---------- */
const pwdVisible = ref(false)
const oldPwd = ref('')
const newPwd = ref('')
const confirmPwd = ref('')
const pwdLoading = ref(false)

function openChangePassword() {
  oldPwd.value = ''
  newPwd.value = ''
  confirmPwd.value = ''
  pwdVisible.value = true
}

async function onConfirmChangePwd() {
  if (!oldPwd.value) return message.warning('请输入原密码')
  if (newPwd.value.length < 6) return message.warning('新密码至少 6 位')
  if (newPwd.value !== confirmPwd.value) return message.warning('两次输入的新密码不一致')
  if (newPwd.value === oldPwd.value) return message.warning('新密码不能与原密码相同')
  pwdLoading.value = true
  try {
    await changeMyPassword({ old_password: oldPwd.value, new_password: newPwd.value })
    message.success('密码修改成功')
    pwdVisible.value = false
    oldPwd.value = newPwd.value = confirmPwd.value = ''
  } catch {
    /* 拦截器已提示 */
  } finally {
    pwdLoading.value = false
  }
}

function onUserSelect(key: string | number) {
  if (key === 'home') router.push('/')
  else if (key === 'logout') {
    store.logout()
    router.replace('/login')
  } else if (key === 'change-password') {
    openChangePassword()
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
        <n-layout-content :native-scrollbar="false">
          <div class="admin-content">
            <router-view />
          </div>
        </n-layout-content>
      </n-layout>
    </n-layout>

    <!-- 修改密码弹窗 -->
    <n-modal :show="pwdVisible" preset="card" title="修改登录密码" style="width: 420px" :bordered="false" @update:show="pwdVisible = $event">
      <n-form label-placement="top">
        <n-form-item label="原密码">
          <n-input v-model:value="oldPwd" type="password" show-password-on="click" placeholder="请输入当前密码" />
        </n-form-item>
        <n-form-item label="新密码">
          <n-input v-model:value="newPwd" type="password" show-password-on="click" placeholder="至少 6 位" />
        </n-form-item>
        <n-form-item label="确认新密码">
          <n-input v-model:value="confirmPwd" type="password" show-password-on="click" placeholder="再次输入新密码" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="pwdVisible = false">取消</n-button>
          <n-button type="primary" :loading="pwdLoading" @click="onConfirmChangePwd">确认修改</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>
