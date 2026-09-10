<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { message } from '@/utils/notify'

const route = useRoute()
const router = useRouter()
const store = useUserStore()

const username = ref('')
const password = ref('')
const loading = ref(false)

/* HC 图标彩蛋：点击 6 次启用管理员隐藏入口 */
let brandClicks = 0
const adminMode = ref(false)

function onBrandClick() {
  brandClicks++
  if (brandClicks >= 6) {
    brandClicks = 0
    adminMode.value = true
    message.success('管理员通道已启用，输入 admin / password 登录即可进入管理面板')
  }
}

onMounted(() => {
  if (store.isLogin) {
    router.replace('/')
    return
  }
  if (route.query.no_admin === '1') {
    message.warning('该账号不是管理员，无权限访问管理后台')
  }
})

async function onSubmit() {
  if (!username.value.trim() || !password.value) {
    message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await store.login(username.value.trim(), password.value)
    // 管理员彩蛋：admin 账号登录 → 直接跳管理后台
    let redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    if (adminMode.value && username.value.trim() === 'admin' && store.isAdmin) {
      redirect = '/admin/monitor'
    }
    if (redirect.startsWith('/admin') && !store.isAdmin) {
      message.error('该账号不是管理员，无权限访问管理后台')
      router.replace('/')
      return
    }
    message.success('登录成功')
    router.replace(redirect)
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-head">
        <div class="brand-mark large" :class="{ 'admin-glow': adminMode }" @click="onBrandClick">
          HC
          <span v-if="adminMode" class="brand-badge">ADMIN</span>
        </div>
        <h1>Happy-Cloud</h1>
        <p>安全、可靠的个人云盘</p>
      </div>
      <n-form label-placement="top" @submit.prevent="onSubmit">
        <n-form-item label="用户名">
          <n-input v-model:value="username" placeholder="请输入用户名" @keyup.enter="onSubmit" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="请输入密码" @keyup.enter="onSubmit" />
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="onSubmit">登 录</n-button>
      </n-form>
      <div class="auth-foot">
        <span>还没有账号？</span>
        <router-link to="/register">立即注册</router-link>
      </div>
    </div>
  </div>
</template>
