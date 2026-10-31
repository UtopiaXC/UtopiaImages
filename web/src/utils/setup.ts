import { ref } from 'vue'
import { api } from '../api/client'

export const isSetupRequired = ref(false)
export const setupChecked = ref(false)

export async function checkSetupStatus(force = false): Promise<boolean> {
  if (setupChecked.value && !force) {
    return isSetupRequired.value
  }
  try {
    const res = await api.get('/setup/status')
    isSetupRequired.value = !!res.data.setup_required
    setupChecked.value = true
    return isSetupRequired.value
  } catch {
    return isSetupRequired.value
  }
}

export function markSetupComplete() {
  isSetupRequired.value = false
  setupChecked.value = true
}

export function resetSetupStatus() {
  setupChecked.value = false
  isSetupRequired.value = false
}
