import { ref } from 'vue'
import { api } from '../api/client'

export const currentUser = ref<any>(null)
export const authInitialized = ref(false)

let pendingPromise: Promise<any> | null = null

export async function fetchCurrentUser(force = false): Promise<any> {
  if (currentUser.value && !force) {
    return currentUser.value
  }
  if (authInitialized.value && !currentUser.value && !force) {
    return null
  }
  if (pendingPromise && !force) {
    return pendingPromise
  }

  pendingPromise = (async () => {
    try {
      const res = await api.get('/me')
      currentUser.value = res.data
      authInitialized.value = true
      return currentUser.value
    } catch {
      currentUser.value = null
      authInitialized.value = true
      return null
    } finally {
      pendingPromise = null
    }
  })()

  return pendingPromise
}

export function setCurrentUser(user: any) {
  currentUser.value = user
  authInitialized.value = true
}

export function clearCurrentUser() {
  currentUser.value = null
  authInitialized.value = true
}
