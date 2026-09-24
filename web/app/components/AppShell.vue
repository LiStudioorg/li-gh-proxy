<script setup lang="ts">
import { Container, Github, Menu, Moon, Rocket, Search, Sun, X, Zap } from 'lucide-vue-next'
import Button from 'fuxsto-design/button'

const { isDark, init, toggle } = useTheme()

const links = [
  { to: '/', label: 'GitHub 加速', icon: Rocket },
  { to: '/images', label: '离线镜像', icon: Container },
  { to: '/search', label: '镜像搜索', icon: Search },
] as const

const route = useRoute()
const menuOpen = ref(false)

function closeMenu() {
  menuOpen.value = false
}

onMounted(() => {
  init()
})
</script>

<template>
  <div class="shell-atmosphere flex min-h-screen flex-col text-foreground">
    <header class="sticky top-0 z-50 border-b border-border/50 bg-background/70 backdrop-blur-xl">
      <div class="mx-auto flex h-[4.25rem] max-w-6xl items-center justify-between gap-3 px-5 sm:px-8">
        <NuxtLink
          to="/"
          class="flex items-center gap-3 text-lg font-semibold tracking-tight transition-opacity hover:opacity-80"
          @click="closeMenu"
        >
          <span class="brand-mark flex size-9 items-center justify-center rounded-lg">
            <Zap class="size-[18px]" />
          </span>
          <span>HubProxy</span>
        </NuxtLink>

        <nav class="hidden items-center gap-1.5 md:flex">
          <NuxtLink
            v-for="link in links"
            :key="link.to"
            :to="link.to"
            class="inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[15px] transition-colors duration-150"
            :class="route.path === link.to ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'"
          >
            <component :is="link.icon" class="size-4" />
            {{ link.label }}
          </NuxtLink>
          <Button
            variant="ghost"
            size="sm"
            class="ml-1 !rounded-full !px-2"
            :aria-label="isDark ? '切换到浅色模式' : '切换到深色模式'"
            @click="toggle"
          >
            <Transition name="fade" mode="out-in">
              <Sun v-if="isDark" key="sun" class="size-4" />
              <Moon v-else key="moon" class="size-4" />
            </Transition>
          </Button>
        </nav>

        <div class="flex items-center gap-0.5 md:hidden">
          <Button
            variant="ghost"
            size="sm"
            class="!rounded-full !px-2"
            :aria-label="isDark ? '切换到浅色模式' : '切换到深色模式'"
            @click="toggle"
          >
            <Transition name="fade" mode="out-in">
              <Sun v-if="isDark" key="sun" class="size-4" />
              <Moon v-else key="moon" class="size-4" />
            </Transition>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="!rounded-full !px-2"
            aria-label="菜单"
            @click="menuOpen = !menuOpen"
          >
            <Transition name="fade" mode="out-in">
              <X v-if="menuOpen" key="x" class="size-4" />
              <Menu v-else key="menu" class="size-4" />
            </Transition>
          </Button>
        </div>
      </div>

      <Transition name="menu">
        <div v-if="menuOpen" class="border-t border-border px-5 py-2 md:hidden">
          <div class="flex flex-col gap-1">
            <NuxtLink
              v-for="link in links"
              :key="link.to"
              :to="link.to"
              class="inline-flex items-center gap-2 rounded-full px-3.5 py-2 text-[15px] transition-colors"
              :class="route.path === link.to ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'"
              @click="closeMenu"
            >
              <component :is="link.icon" class="size-4" />
              {{ link.label }}
            </NuxtLink>
          </div>
        </div>
      </Transition>
    </header>

    <main class="mx-auto w-full max-w-6xl flex-1 px-5 py-10 text-base sm:px-8 sm:py-16">
      <slot />
    </main>

    <footer class="flex justify-center pb-10 pt-2">
      <a
        href="https://github.com/LiStudioorg/li-gh-proxy"
        target="_blank"
        rel="noopener noreferrer"
        aria-label="GitHub"
        class="text-muted-foreground transition-colors duration-150 hover:text-foreground"
      >
        <Github class="size-5" />
      </a>
    </footer>
  </div>
</template>
