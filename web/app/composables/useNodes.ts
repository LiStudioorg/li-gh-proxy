import { fetchNodes, type NodeInfo } from '~/utils/api'

const NODE_STORAGE_KEY = 'node'

// localStorage 在隐私模式/存储被禁用时可能抛异常，统一吞掉仅降级为不记忆选择
function readSavedNode(): string | null {
  try {
    return localStorage.getItem(NODE_STORAGE_KEY)
  } catch {
    return null
  }
}

function persistSelection(url: string) {
  try {
    if (url) localStorage.setItem(NODE_STORAGE_KEY, url)
    else localStorage.removeItem(NODE_STORAGE_KEY)
  } catch {
    // 存储不可用时忽略
  }
}

interface ResolvedNode {
  origin: string
  host: string
}

export function useNodes() {
  const nodes = useState<NodeInfo[]>('accelerator-nodes', () => [])
  const selectedUrl = useState<string>('accelerator-node-url', () => '')

  const currentHost = computed(() => (import.meta.client ? window.location.host : ''))

  const resolved = computed<ResolvedNode>(() => {
    const node = nodes.value.find((n) => n.url === selectedUrl.value)
    if (node) {
      try {
        const u = new URL(node.url)
        return { origin: u.origin, host: u.host }
      } catch {
        // URL 非法时回退当前站点
      }
    }
    return { origin: `https://${currentHost.value}`, host: currentHost.value }
  })

  const origin = computed(() => resolved.value.origin)
  const host = computed(() => resolved.value.host)

  function select(url: string) {
    selectedUrl.value = url
    if (import.meta.client) persistSelection(url)
  }

  async function load() {
    if (import.meta.server) return
    try {
      const res = await fetchNodes()
      nodes.value = res.nodes
      const saved = readSavedNode()
      if (saved && res.nodes.some((n) => n.url === saved)) {
        selectedUrl.value = saved
      } else {
        selectedUrl.value = ''
        if (saved) persistSelection('')
      }
    } catch {
      // 获取失败时静默降级为当前站点
      nodes.value = []
      selectedUrl.value = ''
    }
  }

  return { nodes, selectedUrl, currentHost, origin, host, select, load }
}
