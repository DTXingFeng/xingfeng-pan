import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

const http = axios.create({
  timeout: 30000,
})

let isRedirecting = false

// 响应拦截器：统一处理错误
http.interceptors.response.use(
  (response) => {
    const data = response.data
    if (data.code && data.code !== 200) {
      const msg = data.message || '请求失败'
      ElMessage.error({ message: msg, grouping: true })
      return Promise.reject(new Error(msg))
    }
    return response
  },
  (error) => {
    if (error.response) {
      const status = error.response.status
      if (status === 401) {
        if (!isRedirecting) {
          isRedirecting = true
          const current = router.currentRoute.value.fullPath
          router.push({ path: '/login', query: current !== '/' ? { redirect: current } : {} })
          setTimeout(() => { isRedirecting = false }, 2000)
        }
        return Promise.reject(error)
      }
      const serverMsg = error.response.data?.message
      const fallback = { 400: '请求参数错误', 403: '拒绝访问', 404: '资源不存在', 500: '服务器错误' }
      ElMessage.error({ message: serverMsg || fallback[status] || `请求失败 (${status})`, grouping: true })
    } else if (error.code === 'ECONNABORTED') {
      ElMessage.error({ message: '请求超时', grouping: true })
    } else {
      ElMessage.error({ message: '网络连接异常', grouping: true })
    }
    return Promise.reject(error)
  }
)

export default http
