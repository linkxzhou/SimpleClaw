<template>
  <a-layout-sider
    v-model:collapsed="collapsed"
    :trigger="null"
    collapsible
    :width="220"
    :collapsed-width="64"
    class="sidebar"
    theme="light"
  >
    <div class="sidebar-header">
      <span v-if="!collapsed" class="logo-text">🤖 SimpleClaw</span>
      <span v-else class="logo-icon">🤖</span>
    </div>

    <a-menu
      v-model:selectedKeys="selectedKeys"
      mode="inline"
      @click="handleMenuClick"
    >
      <a-menu-item key="/">
        <template #icon><MessageOutlined /></template>
        <span>{{ t('sidebar.chat') }}</span>
      </a-menu-item>
      <a-menu-item key="/cron">
        <template #icon><ClockCircleOutlined /></template>
        <span>{{ t('sidebar.cronTasks') }}</span>
      </a-menu-item>
      <a-menu-item key="/skills">
        <template #icon><AppstoreOutlined /></template>
        <span>{{ t('sidebar.skills') }}</span>
      </a-menu-item>
      <a-menu-item key="/channels">
        <template #icon><WifiOutlined /></template>
        <span>{{ t('sidebar.channels') }}</span>
      </a-menu-item>
      <a-menu-item key="/dashboard">
        <template #icon><DashboardOutlined /></template>
        <span>{{ t('sidebar.dashboard') }}</span>
      </a-menu-item>
      <a-menu-item key="/settings">
        <template #icon><SettingOutlined /></template>
        <span>{{ t('sidebar.settings') }}</span>
      </a-menu-item>
    </a-menu>

    <div class="sidebar-footer">
      <a-button
        type="text"
        :icon="h(collapsed ? MenuUnfoldOutlined : MenuFoldOutlined)"
        @click="toggleCollapse"
        class="collapse-btn"
      />
    </div>
  </a-layout-sider>
</template>

<script setup lang="ts">
import { ref, computed, watch, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/stores/settings'
import {
  MessageOutlined,
  ClockCircleOutlined,
  AppstoreOutlined,
  WifiOutlined,
  DashboardOutlined,
  SettingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons-vue'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const settings = useSettingsStore()

const collapsed = ref(settings.sidebarCollapsed)
const selectedKeys = computed(() => {
  const path = route.path
  if (path === '/') return ['/']
  const match = ['/', '/cron', '/skills', '/channels', '/dashboard', '/settings'].find(p => path.startsWith(p) && p !== '/')
  return [match || '/']
})

watch(collapsed, (val) => {
  settings.sidebarCollapsed = val
})

function toggleCollapse() {
  collapsed.value = !collapsed.value
}

function handleMenuClick({ key }: { key: string }) {
  router.push(key)
}
</script>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--sidebar-bg) !important;
  backdrop-filter: var(--glass-blur);
  border-right: 1px solid var(--glass-border) !important;
}
.sidebar-header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-color);
  font-weight: 600;
  font-size: 16px;
  color: var(--accent);
  letter-spacing: 0.5px;
}
.logo-text { white-space: nowrap; overflow: hidden; }
.logo-icon { font-size: 24px; }
.sidebar :deep(.ant-menu) {
  background: transparent !important;
  border-inline-end: none !important;
}
.sidebar :deep(.ant-menu-item) {
  border-radius: var(--radius-sm);
  margin: 2px 8px !important;
  transition: all 0.25s;
}
.sidebar :deep(.ant-menu-item:hover) {
  background: var(--bg-hover) !important;
}
.sidebar :deep(.ant-menu-item-selected) {
  background: var(--accent-dim) !important;
  color: var(--accent-light) !important;
  box-shadow: inset 3px 0 0 var(--accent);
}
.sidebar-footer {
  margin-top: auto;
  padding: 8px;
  border-top: 1px solid var(--border-color);
  display: flex;
  justify-content: center;
}
.collapse-btn { width: 100%; }
</style>
