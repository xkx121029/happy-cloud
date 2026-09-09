import { createApp } from 'vue'
import { createPinia } from 'pinia'
import {
  NAvatar, NBreadcrumb, NBreadcrumbItem, NButton, NButtonGroup, NCard, NConfigProvider,
  NDialogProvider, NDrawer, NDrawerContent, NDropdown, NEmpty, NForm, NFormItem,
  NGrid, NGridItem, NIcon, NInput, NInputNumber, NLayout, NLayoutContent, NLayoutHeader,
  NLayoutSider, NMenu, NMessageProvider, NModal, NPagination, NProgress, NSelect,
  NSpace, NSpin, NTag
} from 'naive-ui'
import App from './App.vue'
import router from './router'
import './styles.css'

const app = createApp(App)

const components = [
  NAvatar, NBreadcrumb, NBreadcrumbItem, NButton, NButtonGroup, NCard, NConfigProvider,
  NDialogProvider, NDrawer, NDrawerContent, NDropdown, NEmpty, NForm, NFormItem,
  NGrid, NGridItem, NIcon, NInput, NInputNumber, NLayout, NLayoutContent, NLayoutHeader,
  NLayoutSider, NMenu, NMessageProvider, NModal, NPagination, NProgress, NSelect,
  NSpace, NSpin, NTag
]

for (const c of components) {
  app.component((c as unknown as { name: string }).name, c)
}

app.use(createPinia())
app.use(router)
app.mount('#app')
