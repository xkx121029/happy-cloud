<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { darkTheme, dateZhCN, zhCN, type GlobalThemeOverrides } from 'naive-ui'
import { useSettingsStore, applyTheme } from '@/stores/settings'
import { useUserStore } from '@/stores/user'
import ConnStatus from '@/components/ConnStatus.vue'

const settings = useSettingsStore()
const userStore = useUserStore()

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#0E6A5C',
    primaryColorHover: '#0B564B',
    primaryColorPressed: '#08443C',
    primaryColorSuppl: '#0B564B',
    infoColor: '#0E6A5C',
    successColor: '#16A34A',
    warningColor: '#D97706',
    errorColor: '#DC2626',
    borderRadius: '8px',
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Arial, sans-serif"
  }
}

const naiveDark = computed(() => (settings.theme === 'dark' ? darkTheme : null))

onMounted(() => {
  // 应用持久化的主题
  applyTheme(settings.theme)
  // 登录态下异步从后端同步
  if (userStore.isLogin) settings.loadFromBackend()
})

// 主题变化时实时应用
watch(
  () => settings.theme,
  (t) => applyTheme(t)
)
</script>

<template>
  <n-config-provider :theme="naiveDark" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
        <!-- 常驻连接状态：全局可见，显示当前走 P2P 还是 HTTP -->
        <ConnStatus />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
