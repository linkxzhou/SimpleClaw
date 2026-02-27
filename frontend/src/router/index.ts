import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/setup',
    name: 'Setup',
    component: () => import('@/pages/Setup.vue'),
  },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'Chat',
        component: () => import('@/pages/Chat.vue'),
      },
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/pages/Dashboard.vue'),
      },
      {
        path: 'channels',
        name: 'Channels',
        component: () => import('@/pages/Channels.vue'),
      },
      {
        path: 'skills',
        name: 'Skills',
        component: () => import('@/pages/Skills.vue'),
      },
      {
        path: 'cron',
        name: 'Cron',
        component: () => import('@/pages/Cron.vue'),
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/pages/Settings.vue'),
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
