import { createRouter, createWebHistory } from 'vue-router'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UploadView from '../views/UploadView.vue'
import FilesView from '../views/FilesView.vue'
import TokensView from '../views/TokensView.vue'
import AdminUsersView from '../views/AdminUsersView.vue'
import AdminRolesView from '../views/AdminRolesView.vue'
import AdminPoliciesView from '../views/AdminPoliciesView.vue'
import AdminStatsView from '../views/AdminStatsView.vue'
import AdminFilesView from '../views/AdminFilesView.vue'
import WatermarksView from '../views/WatermarksView.vue'
import UserSettingsView from '../views/UserSettingsView.vue'
import AdminSettingsView from '../views/AdminSettingsView.vue'
import { api, setRouter } from '../api/client'
import { fetchCurrentUser, currentUser } from '../utils/auth'
import { fetchSiteInfo, updateDocumentTitle } from '../utils/site'
import { checkSetupStatus, isSetupRequired } from '../utils/setup'
import { i18n } from '../i18n'

const routes = [
  { path: '/setup', component: SetupView },
  { path: '/login', component: LoginView },
  {
    path: '/console',
    component: DashboardLayout,
    children: [
      { path: '', redirect: '/console/upload' },
      { path: 'upload', component: UploadView },
      { path: 'files', component: FilesView },
      { path: 'watermarks', component: WatermarksView },
      { path: 'tokens', component: TokensView },
      { path: 'settings', component: UserSettingsView },
      { path: 'admin/files', component: AdminFilesView },
      { path: 'admin/users', component: AdminUsersView },
      { path: 'admin/roles', component: AdminRolesView },
      { path: 'admin/policies', component: AdminPoliciesView },
      { path: 'admin/settings', component: AdminSettingsView },
      { path: 'admin/stats', component: AdminStatsView },
    ],
  },
  { path: '/', redirect: '/console/upload' },
  { path: '/:pathMatch(.*)*', redirect: '/console/upload' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

setRouter(router)

let siteInfoFetched = false

// Navigation guard: Authenticate first before deciding which view to render
router.beforeEach(async (to, from, next) => {
  // Always fetch site info initially so title and branding are available
  if (!siteInfoFetched) {
    siteInfoFetched = true
    fetchSiteInfo().catch(() => {})
  }

  // 1. If explicitly navigating to /setup, allow directly
  if (to.path === '/setup') {
    return next()
  }

  // 2. Check if setup is required
  const needsSetup = await checkSetupStatus()
  if (needsSetup) {
    return next('/setup')
  }

  // 3. If navigating to /login
  if (to.path === '/login') {
    // If already logged in, redirect to console
    if (currentUser.value) {
      return next('/console/upload')
    }
    // Otherwise allow viewing login page without triggering /me probe
    return next()
  }

  // 4. For protected console routes (or / redirect)
  const user = currentUser.value || await fetchCurrentUser()
  if (!user) {
    return next({
      path: '/login',
      query: to.path !== '/console/upload' && to.path !== '/' ? { redirect: to.fullPath } : undefined,
    })
  }

  // 5. Check admin role for admin console pages
  if (to.path.startsWith('/console/admin')) {
    const isAdmin = user.role_id === 1 || user.role === 'admin' || (user.role && user.role.name === 'admin')
    if (!isAdmin) {
      return next('/console/upload')
    }
  }

  next()
})

router.afterEach((to) => {
  let title = ''
  // @ts-ignore
  const t = i18n.global.t
  if (to.path.includes('/upload')) title = t('nav.upload')
  else if (to.path.includes('/files')) title = t('nav.files')
  else if (to.path.includes('/watermarks')) title = t('nav.watermarks')
  else if (to.path.includes('/tokens')) title = t('nav.tokens')
  else if (to.path.includes('/settings')) title = t('nav.settings')
  else if (to.path.includes('/admin/files')) title = t('nav.adminFiles')
  else if (to.path.includes('/admin/users')) title = t('nav.adminUsers')
  else if (to.path.includes('/admin/roles')) title = t('nav.adminRoles')
  else if (to.path.includes('/admin/policies')) title = t('nav.adminPolicies')
  else if (to.path.includes('/admin/settings')) title = t('nav.adminSettings')
  else if (to.path.includes('/admin/stats')) title = t('nav.adminStats')
  else if (to.path === '/login') title = t('login.login')
  else if (to.path === '/setup') title = t('setup.title')

  updateDocumentTitle(title)
})
