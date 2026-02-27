<template>
  <div class="mobile-tabbar">
    <div
      v-for="item in tabs"
      :key="item.path"
      :class="['tab-item', { active: isActive(item.path) }]"
      @click="router.push(item.path)"
    >
      <component :is="item.icon" class="tab-icon" />
      <span class="tab-label">{{ t(item.label) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  MessageOutlined,
  AppstoreOutlined,
  WifiOutlined,
  DashboardOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()

const tabs = [
  { path: '/', icon: MessageOutlined, label: 'sidebar.chat' },
  { path: '/skills', icon: AppstoreOutlined, label: 'sidebar.skills' },
  { path: '/channels', icon: WifiOutlined, label: 'sidebar.channels' },
  { path: '/dashboard', icon: DashboardOutlined, label: 'sidebar.dashboard' },
  { path: '/settings', icon: SettingOutlined, label: 'sidebar.settings' },
]

function isActive(path: string): boolean {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}
</script>

<style scoped>
.mobile-tabbar {
  display: flex;
  align-items: center;
  justify-content: space-around;
  height: 56px;
  background: var(--glass-bg);
  backdrop-filter: var(--glass-blur);
  border-top: 1px solid var(--glass-border);
  flex-shrink: 0;
  padding-bottom: env(safe-area-inset-bottom, 0);
}
.tab-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  height: 100%;
  cursor: pointer;
  color: var(--text-tertiary);
  transition: color 0.2s;
  -webkit-tap-highlight-color: transparent;
  position: relative;
}
.tab-item.active {
  color: var(--accent-light);
}
.tab-item.active::after {
  content: '';
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 24px;
  height: 2px;
  background: var(--accent);
  border-radius: 0 0 2px 2px;
  box-shadow: 0 0 8px var(--accent);
}
.tab-icon { font-size: 20px; margin-bottom: 2px; }
.tab-label { font-size: 10px; line-height: 1; }
</style>
