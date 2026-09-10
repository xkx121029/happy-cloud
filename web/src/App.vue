<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { darkTheme, dateZhCN, zhCN, type GlobalThemeOverrides } from 'naive-ui'
import { useSettingsStore, applyTheme } from '@/stores/settings'
import { useUserStore } from '@/stores/user'

const settings = useSettingsStore()
const userStore = useUserStore()

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#2563EB',
    primaryColorHover: '#3B82F6',
    primaryColorPressed: '#1D4ED8',
    primaryColorSuppl: '#3B82F6',
    infoColor: '#2563EB',
    successColor: '#16A34A',
    warningColor: '#D97706',
    errorColor: '#DC2626',
    borderRadius: '6px',
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
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
