import { createRouter, createWebHistory } from 'vue-router'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UploadView from '../views/UploadView.vue'
import FilesView from '../views/FilesView.vue'
import TokensView from '../views/TokensView.vue'
import AdminUsersView from '../views/AdminUsersView.vue'
import AdminPoliciesView from '../views/AdminPoliciesView.vue'
import AdminStatsView from '../views/AdminStatsView.vue'
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
      { path: 'tokens', component: TokensView },
      { path: 'admin/users', component: AdminUsersView },
      { path: 'admin/policies', component: AdminPoliciesView },
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

  next()
})
