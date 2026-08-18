import { createRouter, createWebHistory } from 'vue-router'
import FileList from '../views/FileList.vue'
import Trash from '../views/Trash.vue'
import Admin from '../views/Admin.vue'
import Login from '../views/Login.vue'
import http from '../utils/request.js'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
  },
  {
    path: '/',
    redirect: '/files',
  },
  {
    path: '/files',
    name: 'Files',
    component: FileList,
  },
  {
    path: '/files/:pathMatch(.*)*',
    name: 'FilesPath',
    component: FileList,
  },
  {
    path: '/trash',
    name: 'Trash',
    component: Trash,
  },
  {
    path: '/admin',
    name: 'Admin',
    component: Admin,
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

const whiteList = ['/login']
let authCache = { authenticated: false, time: 0 }
const AUTH_CACHE_TTL = 5 * 60 * 1000

router.beforeEach(async (to) => {
  if (whiteList.includes(to.path)) {
    return true
  }

  const now = Date.now()
  if (authCache.authenticated && now - authCache.time < AUTH_CACHE_TTL) {
    return true
  }

  try {
    const res = await http.get('/api/auth/check')
    if (res.data.data?.authenticated) {
      authCache = { authenticated: true, time: now }
      return true
    }
    return { path: '/login', query: { redirect: to.fullPath } }
  } catch {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
})

export function clearAuthCache() {
  authCache = { authenticated: false, time: 0 }
}

export default router
