import axios from 'axios'
import { i18n } from '../i18n'

export const api = axios.create({
  baseURL: '/api/v1',
  withCredentials: true,
})

export function getApiErrorMessage(error: any): string {
  const code = error?.response?.data?.code || error?.response?.data?.error
  if (code && typeof code === 'string') {
    const key = `errors.${code}`
    // @ts-ignore
    if (i18n.global.te(key)) {
      // @ts-ignore
      return i18n.global.t(key)
    }
  }
  return error?.response?.data?.message || error?.response?.data?.error || error?.message || 'Operation failed'
}

let routerInstance: any = null

export function setRouter(router: any) {
  routerInstance = router
}

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      const url = error.config?.url || ''
      // Never intercept auth endpoints, login attempts, setup calls, or probe endpoints
      const isAuthOrSetupEndpoint =
        url.includes('/me') ||
        url.includes('/auth/') ||
        url.includes('/setup') ||
        url.includes('/site')

      const currentPath = routerInstance?.currentRoute?.value?.path || window.location.pathname
      const isPublicView = currentPath === '/login' || currentPath === '/setup'

      if (!isAuthOrSetupEndpoint && !isPublicView) {
        if (routerInstance) {
          routerInstance.push('/login')
        } else {
          window.location.href = '/login'
        }
      }
    }
    return Promise.reject(error)
  }
)
