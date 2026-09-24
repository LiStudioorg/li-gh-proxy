export function useTheme() {
  const isDark = useState('theme-dark', () => false)

  function apply(dark: boolean) {
    isDark.value = dark
    if (import.meta.client) {
      document.documentElement.classList.toggle('dark', dark)
      localStorage.setItem('theme', dark ? 'dark' : 'light')
    }
  }

  function init() {
    if (import.meta.client) {
      const saved = localStorage.getItem('theme')
      apply(
        saved === 'dark' || saved === 'light'
          ? saved === 'dark'
          : window.matchMedia('(prefers-color-scheme: dark)').matches,
      )
    }
  }

  function toggle() {
    apply(!isDark.value)
  }

  return { isDark, init, toggle }
}
