<script setup lang="ts">
// 常驻连接状态指示：始终显示当前数据链路是「P2P 直连」还是「HTTP 直连」。
// 点击可一键切换（建链 / 断链），无需进入设置抽屉。
import { computed, watch } from 'vue'
import { useP2P } from '@/p2p'
import { useSettingsStore } from '@/stores/settings'
import { useUserStore } from '@/stores/user'

const p2p = useP2P()
const settings = useSettingsStore()
const userStore = useUserStore()

const label = computed(() => {
  if (p2p.state.busy) return '连接中'
  return p2p.state.connected ? 'P2P 直连' : 'HTTP 直连'
})

const tone = computed(() => {
  if (p2p.state.busy) return 'busy'
  return p2p.state.connected ? 'on' : 'off'
})

const detail = computed(() => {
  const base = p2p.state.connected
    ? '数据经 WebRTC 隧道直连服务器（打洞成功）'
    : '数据经 HTTP 直连服务器'
  return p2p.state.error ? `${base}｜${p2p.state.error}` : base
})

async function onToggle() {
  if (p2p.state.connected) {
    p2p.disconnect()
    await settings.set('p2pEnabled', false)
    return
  }
  await settings.set('p2pEnabled', true)
  await p2p.connect()
}

// 登录态变化时同步：登录且开启 P2P 则自动建链，登出则断链
watch(
  () => userStore.isLogin,
  (login) => {
    if (login) {
      if (settings.p2pEnabled) p2p.connect()
    } else {
      p2p.disconnect()
    }
  },
  { immediate: true }
)

// 设置从后端加载后再开启 P2P 时补建链
watch(
  () => settings.p2pEnabled,
  (on) => {
    if (on && userStore.isLogin && !p2p.state.connected && !p2p.state.busy) p2p.connect()
  }
)
</script>

<template>
  <button v-if="userStore.isLogin" class="conn-badge" :class="`conn-${tone}`" :title="detail" @click="onToggle">
    <span class="conn-dot" />
    <span class="conn-text">{{ label }}</span>
  </button>
</template>