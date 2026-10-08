import { computed, reactive } from 'vue'
import { api, auth } from './api'

export const state = reactive({
  booting: true,
  online: false,
  user: null,
  ownedLibraries: [],
  publicLibraries: [],
  usage: null,
  config: {},
  toast: null,
})

export const allLibraries = computed(() => [
  ...state.ownedLibraries.map((item) => ({ ...item, readOnly: false })),
  ...state.publicLibraries.map((item) => ({ ...item, readOnly: true })),
])

let toastTimer = null

export function notify(text, kind = 'ok') {
  state.toast = { id: Date.now(), text, kind }
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { state.toast = null }, kind === 'bad' ? 5200 : 3000)
}

export async function bootstrap() {
  state.booting = true
  try {
    await api.health()
    state.online = true
    if (auth.refreshToken()) await loadWorkspace()
  } catch (error) {
    state.online = false
    if (error.code !== 'SESSION_INVALID') notify(error.message, 'bad')
    auth.clear()
  } finally {
    state.booting = false
  }
}

export async function completeLogin(result) {
  auth.accept(result)
  await loadWorkspace()
}

export async function loadWorkspace() {
  const [user, libraries, usage, config] = await Promise.all([
    api.me(), api.libraries({ limit: 100 }), api.usageSummary(), api.config().catch(() => ({})),
  ])
  state.user = user
  state.ownedLibraries = libraries.owned || []
  state.publicLibraries = libraries.public || []
  state.usage = usage
  state.config = config || {}
  state.online = true
  return state
}

export async function logout() {
  try { await api.logout() } finally {
    auth.clear()
    state.user = null
    state.ownedLibraries = []
    state.publicLibraries = []
    state.usage = null
  }
}
