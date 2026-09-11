import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/user/Login.vue'), meta: { public: true } },
    { path: '/register', name: 'register', component: () => import('@/views/user/Register.vue'), meta: { public: true } },
    { path: '/share/:token', name: 'share', component: () => import('@/views/SharePreview.vue'), meta: { public: true } },
    { path: '/', name: 'home', component: () => import('@/views/user/Home.vue') },
    { path: '/shared', name: 'shared', component: () => import('@/views/Shared.vue') },
    { path: '/notifications', name: 'notifications', component: () => import('@/views/Notifications.vue') },
    { path: '/trash', name: 'trash', component: () => import('@/views/user/Trash.vue') },
    { path: '/shares', name: 'shares', component: () => import('@/views/user/Shares.vue') },
    {
      path: '/admin',
      component: () => import('@/views/admin/AdminLayout.vue'),
      redirect: '/admin/users',
      children: [
        { path: 'users', name: 'admin-users', component: () => import('@/views/admin/Users.vue') },
        { path: 'files', name: 'admin-files', component: () => import('@/views/admin/Files.vue') },
        { path: 'monitor', name: 'admin-monitor', component: () => import('@/views/admin/Monitor.vue') },
        { path: 'logs', name: 'admin-logs', component: () => import('@/views/admin/Logs.vue') }
      ]
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

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
      return { path: '/login', query: { redirect: to.fullPath, no_admin: '1' } }
    }
  }
  return true
})

export default router
