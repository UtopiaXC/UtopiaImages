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
import { api } from '../api/client'

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

// Navigation guard
router.beforeEach(async (to, from, next) => {
  if (to.path === '/setup') {
    return next()
  }

  try {
    const res = await api.get('/setup/status')
    if (res.data.setup_required) {
      return next('/setup')
    }
  } catch {}

  if (to.path.startsWith('/console/admin')) {
    try {
      const meRes = await api.get('/me')
      const user = meRes.data
      const isAdmin = user.role_id === 1 || user.role?.name === 'admin'
      if (!isAdmin) {
        return next('/console/upload')
      }
    } catch {
      return next('/login')
    }
  }

  next()
})
