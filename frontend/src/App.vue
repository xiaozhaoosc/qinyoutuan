<template>
  <n-config-provider :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { NConfigProvider, NMessageProvider, NDialogProvider, zhCN, dateZhCN } from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import { useConfigStore } from '@/stores/config'

const themeOverrides: GlobalThemeOverrides = {
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

// 只拉站点配置；auth 由 router guard 负责初始化
const config = useConfigStore()
config.fetchConfig()
</script>
