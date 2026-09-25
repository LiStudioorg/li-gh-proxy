import { fetchFeatures, type FeaturesResponse } from '~/utils/api'

const DEFAULT_FEATURES: FeaturesResponse = { friends: false, sponsors: false }

export function useFeatures() {
  // useState 跨组件共享：AppShell 与页面共用同一次请求结果
  const features = useState<FeaturesResponse>('site-features', () => ({ ...DEFAULT_FEATURES }))
  const loaded = useState<boolean>('site-features-loaded', () => false)

  async function load() {
    if (import.meta.server || loaded.value) return
    loaded.value = true
    try {
      features.value = await fetchFeatures()
    } catch {
      // 接口不可用（旧版本后端等）时按全部关闭处理
      features.value = { ...DEFAULT_FEATURES }
    }
  }

  return { features, load }
}
