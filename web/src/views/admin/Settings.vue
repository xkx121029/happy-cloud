<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { fetchSettings, saveSettings } from '@/api/admin'
import type { SystemSettings } from '@/api/types'
import { message } from '@/utils/notify'

const loading = ref(false)
const saving = ref(false)
const form = ref({
  allow_register: true,
  default_quota_gb: 10,
  max_upload_mb: 100,
  public_url: '',
  p2p_public_host: ''
})

async function load() {
  loading.value = true
  try {
    const s: SystemSettings = await fetchSettings()
    form.value.allow_register = s.allow_register === 'true'
    form.value.default_quota_gb = Math.round(Number(s.default_quota || 0) / (1024 * 1024 * 1024)) || 10
    form.value.max_upload_mb = Number(s.max_upload_mb || 100)
    form.value.public_url = s.public_url || ''
    form.value.p2p_public_host = s.p2p_public_host || ''
  } catch {
    /* 拦截器已提示 */
  } finally {
    loading.value = false
  }
}

async function onSave() {
  if (!form.value.default_quota_gb || form.value.default_quota_gb < 0) return message.warning('默认配额需为非负整数（GB）')
  if (!form.value.max_upload_mb || form.value.max_upload_mb <= 0) return message.warning('上传上限需为正整数（MB）')
  if (form.value.public_url && !/^https?:\/\//.test(form.value.public_url)) return message.warning('主页面网址需以 http:// 或 https:// 开头')
  if (form.value.p2p_public_host && /[:/]/.test(form.value.p2p_public_host)) return message.warning('P2P 隧道主机需为域名或 IP（不含协议与端口）')
  saving.value = true
  try {
    await saveSettings({
      allow_register: String(form.value.allow_register),
      default_quota: String(form.value.default_quota_gb * 1024 * 1024 * 1024),
      max_upload_mb: String(form.value.max_upload_mb),
      public_url: form.value.public_url.trim(),
      p2p_public_host: form.value.p2p_public_host.trim()
    })
    message.success('设置已保存')
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="page-toolbar">
      <div class="page-title">系统设置</div>
      <div class="toolbar-right">
        <n-button :loading="saving" type="primary" @click="onSave">保存设置</n-button>
      </div>
    </div>

    <n-card size="small" title="全局设置">
      <n-spin :show="loading">
        <n-form label-placement="left" label-width="180" class="settings-form">
          <n-form-item label="开放注册">
            <n-switch v-model:value="form.allow_register">
              <template #checked>开放</template>
              <template #unchecked>关闭</template>
            </n-switch>
            <div class="form-tip">关闭后用户无法自助注册，仅保留登录</div>
          </n-form-item>
          <n-form-item label="新用户默认配额 (GB)">
            <n-input-number v-model:value="form.default_quota_gb" :min="0" :step="1" style="width: 220px" />
            <div class="form-tip">新注册用户的初始存储配额，单位 GB</div>
          </n-form-item>
          <n-form-item label="单文件上传上限 (MB)">
            <n-input-number v-model:value="form.max_upload_mb" :min="1" :step="1" style="width: 220px" />
            <div class="form-tip">超过该阈值的文件需通过分片上传</div>
          </n-form-item>
          <n-form-item label="主页面网址">
            <n-input v-model:value="form.public_url" placeholder="如 https://cloud.example.com（留空则使用当前访问地址）" clearable />
            <div class="form-tip">公开访问主页面（web）的完整网址，用于生成分享链接等对外绝对地址</div>
          </n-form-item>
          <n-form-item label="P2P 隧道主机">
            <n-input v-model:value="form.p2p_public_host" placeholder="如 cloud.example.com（留空则自动按访问域名推导）" clearable />
            <div class="form-tip">P2P 打洞下发的信令/STUN 对外主机（不含协议与端口）。cloudflared 等隧道穿透时需填写与隧道对应的公网主机</div>
          </n-form-item>
        </n-form>
      </n-spin>
    </n-card>
  </div>
</template>

<style scoped>
.settings-form {
  max-width: 640px;
  margin-top: 8px;
}
.form-tip {
  width: 100%;
  font-size: 12px;
  color: var(--n-text-color-3);
  line-height: 1.6;
}
</style>
