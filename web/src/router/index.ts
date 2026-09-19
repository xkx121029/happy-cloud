import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

const APP_TITLE = 'Happy-Cloud 云盘'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior() {
    // 路由切换回到顶部（保留浏览器前进/后退的滚动位置）
    return { top: 0 }
  },
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/user/Login.vue'), meta: { public: true, title: '登录' } },
    { path: '/register', name: 'register', component: () => import('@/views/user/Register.vue'), meta: { public: true, title: '注册' } },
    { path: '/share/:token', name: 'share', component: () => import('@/views/SharePreview.vue'), meta: { public: true, title: '文件分享' } },
    { path: '/', name: 'home', component: () => import('@/views/user/Home.vue'), meta: { title: '全部文件' } },
    { path: '/browse/:folderId?', name: 'browse', component: () => import('@/views/user/Home.vue'), meta: { title: '文件浏览' } },
    { path: '/shared', name: 'shared', component: () => import('@/views/Shared.vue'), meta: { title: '共享目录' } },
    { path: '/notifications', name: 'notifications', component: () => import('@/views/Notifications.vue'), meta: { title: '通知' } },
    { path: '/trash', name: 'trash', component: () => import('@/views/user/Trash.vue'), meta: { title: '回收站' } },
    { path: '/shares', name: 'shares', component: () => import('@/views/user/Shares.vue'), meta: { title: '我的分享' } },
    {
      path: '/admin',
      component: () => import('@/views/admin/AdminLayout.vue'),
      redirect: '/admin/users',
      meta: { title: '管理后台' },
      children: [
        { path: 'users', name: 'admin-users', component: () => import('@/views/admin/Users.vue'), meta: { title: '用户管理' } },
        { path: 'files', name: 'admin-files', component: () => import('@/views/admin/Files.vue'), meta: { title: '文件管理' } },
        { path: 'monitor', name: 'admin-monitor', component: () => import('@/views/admin/Monitor.vue'), meta: { title: '系统监控' } },
        { path: 'logs', name: 'admin-logs', component: () => import('@/views/admin/Logs.vue'), meta: { title: '操作日志' } },
        { path: 'storage', name: 'admin-storage', component: () => import('@/views/admin/Storage.vue'), meta: { title: '存储管理' } }
      ]
    },
    // 兜底：未匹配路由进入 404 页面（而非静默跳回首页）
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFound.vue'), meta: { public: true, title: '页面不存在' } }
  ]
})

// 全局守卫：公开页放行；其余需登录；/admin 需管理员角色
router.beforeEach(async (to) => {
  const store = useUserStore()
  if (to.meta.public) return true
  if (!store.token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.path.startsWith('/admin')) {
    if (!store.user) {
      try {
        await store.loadMe()
      } catch {
        store.logout()
        return { path: '/login', query: { redirect: to.fullPath } }
      }
    }
    if (store.user?.role !== 1) {
      // 非管理员访问后台 → 回到首页，避免误导性登录跳转
      return { path: '/' }
    }
  }
  return true
})

// 设置页面标题
router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} · ${APP_TITLE}` : APP_TITLE
})

export default router
