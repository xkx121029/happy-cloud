<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { message } from '@/utils/notify'

const router = useRouter()
const store = useUserStore()

const username = ref('')
const password = ref('')
const email = ref('')
const loading = ref(false)

async function onSubmit() {
  if (!username.value.trim() || !password.value) {
    message.warning('请输入用户名和密码')
    return
  }
  if (password.value.length < 6) {
    message.warning('密码长度至少 6 位')
    return
  }
  loading.value = true
  try {
    await store.register(username.value.trim(), password.value, email.value.trim())
    message.success('注册成功，已自动登录')
    router.replace('/')
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
        <div class="brand-mark large">HC</div>
        <h1>注册账号</h1>
        <p>加入 Happy-Cloud，随时存取你的文件</p>
      </div>
      <n-form label-placement="top" @submit.prevent="onSubmit">
        <n-form-item label="用户名">
          <n-input v-model:value="username" placeholder="请输入用户名" @keyup.enter="onSubmit" />
        </n-form-item>
        <n-form-item label="邮箱">
          <n-input v-model:value="email" placeholder="选填，用于找回账号" @keyup.enter="onSubmit" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="至少 6 位" @keyup.enter="onSubmit" />
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="onSubmit">注 册</n-button>
      </n-form>
      <div class="auth-foot">
        <span>已有账号？</span>
        <router-link to="/login">返回登录</router-link>
      </div>
    </div>
  </div>
</template>
