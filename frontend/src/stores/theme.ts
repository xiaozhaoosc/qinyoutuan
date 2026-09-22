import { defineStore } from 'pinia'
import { ref } from 'vue'

// 主题模式：跟随系统 / 手动亮 / 手动暗
// 持久化到 localStorage，并给 <html> 挂 'dark' class，驱动 global.css 的深浅 CSS 变量。
export type ThemeMode = 'light' | 'dark' | 'auto'

const KEY = 'qz-theme'

// 系统当前是否为深色（仅在 auto 模式下有意义）
function systemDark(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>((localStorage.getItem(KEY) as ThemeMode) || 'auto')
  // resolved 是实际应用的主题（true=深色），naive 的 darkTheme 与 CSS 变量都看它
  const resolved = ref(false)

  function apply() {
    const dark = mode.value === 'dark' || (mode.value === 'auto' && systemDark())
    resolved.value = dark
    if (typeof document !== 'undefined') {
      document.documentElement.classList.toggle('dark', dark)
      // 供 CSS / 个别内联样式用
      document.documentElement.dataset.theme = dark ? 'dark' : 'light'
      // 覆盖浏览器默认背景，避免滚动条区域在深色下闪白
      document.body.style.colorScheme = dark ? 'dark' : 'light'
    }
  }

  function setMode(m: ThemeMode) {
    mode.value = m
    try { localStorage.setItem(KEY, m) } catch {}
    apply()
  }

  // 手动在亮/暗之间切换（顶栏按钮用），会把 auto 冻结成显式选择
  function toggle() {
    setMode(resolved.value ? 'light' : 'dark')
  }

  // auto 模式下监听系统切换实时跟随
  let mql: MediaQueryList | null = null
  const onChange = () => { if (mode.value === 'auto') apply() }
  if (typeof window !== 'undefined') {
    mql = window.matchMedia('(prefers-color-scheme: dark)')
    mql.addEventListener('change', onChange)
  }

  apply()
  // 返回清理函数，避免 store 在测试/热更等场景重复监听
  const dispose = () => { mql?.removeEventListener('change', onChange) }

  return { mode, resolved, setMode, toggle, dispose }
})