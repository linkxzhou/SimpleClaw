<template>
  <a-layout class="main-layout">
    <!-- Desktop sidebar -->
    <Sidebar v-if="!settings.isMobile" />

    <a-layout class="content-layout">
      <!-- Mobile top bar -->
      <div v-if="settings.isMobile" class="mobile-topbar">
        <span class="mobile-logo">🤖 SimpleClaw</span>
      </div>

      <a-layout-content class="main-content" :class="{ 'mobile-content': settings.isMobile }">
        <router-view />
      </a-layout-content>

      <!-- Mobile bottom tab bar -->
      <MobileTabBar v-if="settings.isMobile" />
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { useSettingsStore } from '@/stores/settings'
import Sidebar from './Sidebar.vue'
import MobileTabBar from './MobileTabBar.vue'

const settings = useSettingsStore()
</script>

<style scoped>
.main-layout { height: 100%; width: 100%; overflow: hidden; }
.content-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  flex: 1;
  min-width: 0;
}
.main-content {
  padding: 24px;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
  background: var(--bg-secondary);
}
.mobile-content { padding: 12px; padding-bottom: 0; }
.mobile-topbar {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--glass-bg);
  backdrop-filter: var(--glass-blur);
  border-bottom: 1px solid var(--glass-border);
  flex-shrink: 0;
  color: var(--accent);
}
.mobile-logo { font-size: 16px; font-weight: 600; letter-spacing: 0.5px; }
</style>
