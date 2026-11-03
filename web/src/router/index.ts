import { createRouter, createWebHistory } from 'vue-router'
const SetupView = () => import('../views/SetupView.vue')
const LoginView = () => import('../views/LoginView.vue')
const DashboardLayout = () => import('../views/DashboardLayout.vue')
const UploadView = () => import('../views/UploadView.vue')
const FilesView = () => import('../views/FilesView.vue')
const TokensView = () => import('../views/TokensView.vue')
const AdminUsersView = () => import('../views/AdminUsersView.vue')
const AdminRolesView = () => import('../views/AdminRolesView.vue')
const AdminPoliciesView = () => import('../views/AdminPoliciesView.vue')
const AdminStatsView = () => import('../views/AdminStatsView.vue')
const AdminFilesView = () => import('../views/AdminFilesView.vue')
const WatermarksView = () => import('../views/WatermarksView.vue')
const AdminWatermarksView = () => import('../views/AdminWatermarksView.vue')
const UserSettingsView = () => import('../views/UserSettingsView.vue')
const AdminSettingsView = () => import('../views/AdminSettingsView.vue')
const GuestUploadView = () => import('../views/GuestUploadView.vue')
const SquareView = () => import('../views/SquareView.vue')
const GuestGalleryView = () => import('../views/GuestGalleryView.vue')

import { api, setRouter } from '../api/client'
import { fetchCurrentUser, currentUser, fetchUserPreferences } from '../utils/auth'
import { fetchSiteInfo, updateDocumentTitle, siteInfo } from '../utils/site'
import { checkSetupStatus, isSetupRequired } from '../utils/setup'
import { i18n } from '../i18n'

const routes = [
  { path: '/stp', component: SetupView },
  { path: '/setup', redirect: '/stp' },

  { path: '/usr/login', component: LoginView },
  { path: '/login', redirect: '/usr/login' },

  { path: '/usr/upload', component: GuestUploadView },
  { path: '/upload', redirect: '/usr/upload' },

  { path: '/usr/square', component: SquareView },
  { path: '/square', redirect: '/usr/square' },

  { path: '/usr/gallery', component: GuestGalleryView },
  { path: '/gallery', redirect: '/usr/gallery' },

  {
    path: '/dsh',
    component: DashboardLayout,
    children: [
      { path: '', redirect: '/dsh/upload' },
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
      { path: 'admin/watermarks', component: AdminWatermarksView },
      { path: 'admin/stats', component: AdminStatsView },
    ],
  },
  {
    path: '/console/:pathMatch(.*)*',
    redirect: (to: any) => `/dsh/${to.params.pathMatch || 'upload'}`,
  },
  { path: '/console', redirect: '/dsh/upload' },

  {
    path: '/',
    component: { render: () => null },
  },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

setRouter(router)

// Navigation guard
router.beforeEach(async (to, from, next) => {
  const site = await fetchSiteInfo().catch(() => siteInfo.value)

  // 1. Setup check
  if (to.path === '/stp' || to.path === '/setup') {
    return next()
  }
  const needsSetup = await checkSetupStatus()
  if (needsSetup) {
    return next('/stp')
  }

  // 2. Public views check
  const publicPaths = ['/usr/upload', '/usr/square', '/usr/gallery', '/upload', '/square', '/gallery']
  if (publicPaths.includes(to.path)) {
    if ((to.path === '/usr/upload' || to.path === '/upload') && !site.enable_guest_upload && !currentUser.value) {
      return next('/usr/login')
    }
    if ((to.path === '/usr/gallery' || to.path === '/gallery') && !site.enable_guest_upload && !currentUser.value) {
      return next('/usr/login')
    }
    if ((to.path === '/usr/square' || to.path === '/square') && !site.enable_square && !currentUser.value) {
      return next('/usr/login')
    }
    return next()
  }

  // 3. Login view check
  if (to.path === '/usr/login' || to.path === '/login') {
    if (currentUser.value) {
      return next('/dsh/upload')
    }
    return next()
  }

  // 4. Root / path redirection
  if (to.path === '/') {
    // If guest upload or square is enabled
    if (site.enable_guest_upload || site.enable_square) {
      // Check user preference if user is logged in
      const user = currentUser.value || await fetchCurrentUser(true).catch(() => null)
      if (user) {
        const prefs = await fetchUserPreferences().catch(() => null)
        const userLanding = prefs?.default_landing_view || 'visitor'
        if (userLanding === 'console') {
          return next('/dsh/upload')
        }
      }
      // Follow system default_open_page
      const defPage = site.default_open_page
      if (defPage === 'guest_upload' && site.enable_guest_upload) {
        return next('/usr/upload')
      }
      if (defPage === 'guest_gallery' && site.enable_guest_upload) {
        return next('/usr/gallery')
      }
      if (defPage === 'square' && site.enable_square) {
        return next('/usr/square')
      }
      if (defPage === 'login') {
        return next('/usr/login')
      }
      // Fallback
      if (site.enable_square) {
        return next('/usr/square')
      }
      if (site.enable_guest_upload) {
        return next('/usr/upload')
      }
      return next('/usr/login')
    }

    if (currentUser.value) {
      return next('/dsh/upload')
    }
    const user = await fetchCurrentUser().catch(() => null)
    if (user) {
      return next('/dsh/upload')
    }
    return next('/usr/login')
  }

  // 5. Protected console routes
  const user = currentUser.value || await fetchCurrentUser(true)
  if (!user) {
    return next({
      path: '/usr/login',
      query: to.path !== '/dsh/upload' && to.path !== '/' ? { redirect: to.fullPath } : undefined,
    })
  }

  // 6. Admin routes check
  if (to.path.startsWith('/dsh/admin') || to.path.startsWith('/console/admin')) {
    const isAdmin = user.role_id === 1 || user.role === 'admin' || (user.role && user.role.name === 'admin')
    if (!isAdmin) {
      return next('/dsh/upload')
    }
  }

  next()
})

router.afterEach((to) => {
  let title = ''
  // @ts-ignore
  const t = i18n.global.t
  if (to.path.includes('/admin/watermarks')) title = t('adminWatermarks.title')
  else if (to.path.includes('/upload')) title = t('nav.upload')
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
  else if (to.path.includes('/login')) title = t('login.login')
  else if (to.path.includes('/setup') || to.path.includes('/stp')) title = t('setup.title')
  else if (to.path.includes('/square')) title = t('square.title')
  else if (to.path.includes('/gallery')) title = t('guest.galleryTitle')

  updateDocumentTitle(title)
})
