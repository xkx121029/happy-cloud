<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
</script>

<template>
  <div class="notfound-page">
    <div class="nf-mark">404</div>
    <h2>页面不存在</h2>
    <p>你访问的地址不存在或已被移除。</p>
    <n-button type="primary" @click="router.push('/')">返回首页</n-button>
  </div>
</template>

<style scoped>
.notfound-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: #f7f8fa;
  color: #333;
}
.nf-mark {
  font-size: 64px;
  font-weight: 800;
  color: #d0d5e0;
  letter-spacing: 2px;
}
h2 { margin: 0; font-size: 20px; }
p { margin: 0; color: #6b7280; font-size: 14px; }
</style>
