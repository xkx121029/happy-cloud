<script setup lang="ts">
import { SettingsOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '@/stores/settings'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const settings = useSettingsStore()

const themes = [
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' }
]
const views = [
  { label: '网格', value: 'grid' },
  { label: '列表', value: 'list' }
]
const sorts = [
  { label: '按名称', value: 'name' },
  { label: '按时间', value: 'date' },
  { label: '按大小', value: 'size' }
]

function close() {
  emit('update:visible', false)
}
</script>

<template>
  <n-drawer :show="visible" placement="right" :width="380" :bordered="false" @update:show="emit('update:visible', $event)">
    <div class="drawer-head">
      <div class="drawer-title">个性化设置</div>
    </div>

    <div class="settings-section">
      <div class="settings-section-title">外观</div>
      <div class="setting-row">
        <span class="setting-label">界面主题</span>
        <n-radio-group :value="settings.theme" size="small" @update:value="(v: any) => settings.set('theme', v)">
          <n-radio-button v-for="t in themes" :key="t.value" :value="t.value">{{ t.label }}</n-radio-button>
        </n-radio-group>
      </div>
    </div>

    <div class="settings-section">
      <div class="settings-section-title">文件浏览</div>
      <div class="setting-row">
        <span class="setting-label">默认视图</span>
        <n-radio-group :value="settings.defaultView" size="small" @update:value="(v: any) => settings.set('defaultView', v)">
          <n-radio-button v-for="v in views" :key="v.value" :value="v.value">{{ v.label }}</n-radio-button>
        </n-radio-group>
      </div>
      <div class="setting-row">
        <span class="setting-label">默认排序</span>
        <n-select :value="settings.defaultSort" :options="sorts" size="small" style="width: 110px" @update:value="(v: any) => settings.set('defaultSort', v)" />
      </div>
      <div class="setting-row">
        <span class="setting-label">每页数量</span>
        <n-input-number v-model:value="settings.pageSize" :min="10" :max="200" :step="10" size="small" style="width: 110px" @update:value="(v: number | null) => v && settings.set('pageSize', v)" />
      </div>
    </div>

    <div class="settings-section">
      <div class="settings-section-title">上传</div>
      <div class="setting-row">
        <span class="setting-label">并行上传数</span>
        <n-input-number v-model:value="settings.uploadConcurrency" :min="1" :max="8" size="small" style="width: 110px" @update:value="(v: number | null) => v && settings.set('uploadConcurrency', v)" />
      </div>
    </div>

    <div class="settings-section settings-footer">
      <div class="settings-section-title">说明</div>
      <p class="settings-tip">设置自动保存到浏览器本地；登录后会同步到服务器，换设备登录也能保留。</p>
    </div>

    <div class="drawer-foot">
      <n-button quaternary @click="close">关闭</n-button>
      <n-button type="primary" :icon="SettingsOutline" circle @click="close" />
    </div>
  </n-drawer>
</template>
