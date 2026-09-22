<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NConfigProvider, NMessageProvider, NDialogProvider, zhCN, dateZhCN, darkTheme, lightTheme } from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import { useConfigStore } from '@/stores/config'
import { useThemeStore } from '@/stores/theme'

const lightOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#3b6cf0',
    primaryColorHover: '#2f5ae0',
    primaryColorPressed: '#274bc0',
    primaryColorSuppl: '#3b6cf0',
    infoColor: '#3b6cf0',
    successColor: '#16a34a',
    warningColor: '#d97706',
    errorColor: '#e5484d',
    borderRadius: '10px',
    borderColor: 'rgba(23, 37, 84, .1)',
    textColorBase: '#0f172a',
    fontFamily: '"Segoe UI Variable", "Segoe UI", "Microsoft YaHei", "PingFang SC", system-ui, sans-serif',
  },
}

const darkOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#5c8cff',
    primaryColorHover: '#6f9aff',
    primaryColorPressed: '#4877e8',
    primaryColorSuppl: '#5c8cff',
    infoColor: '#5c8cff',
    successColor: '#3fbf6e',
    warningColor: '#f0a13c',
    errorColor: '#f2676b',
    borderRadius: '10px',
    borderColor: 'rgba(148, 163, 184, .14)',
    textColorBase: '#e2e8f0',
    fontFamily: '"Segoe UI Variable", "Segoe UI", "Microsoft YaHei", "PingFang SC", system-ui, sans-serif',
  },
}

const theme = useThemeStore()
const naiveTheme = computed(() => (theme.resolved ? darkTheme : lightTheme))
const themeOverrides = computed(() => (theme.resolved ? darkOverrides : lightOverrides))

// 只拉站点配置；auth 由 router guard 负责初始化
const config = useConfigStore()
config.fetchConfig()
</script>
