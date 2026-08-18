<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="login-title">刑风网盘</h1>
      <p class="login-subtitle">请输入访问令牌</p>
      <el-input
        ref="tokenInput"
        v-model="token"
        type="password"
        placeholder="请输入访问令牌"
        show-password
        @keyup.enter="handleLogin"
        size="large"
      />
      <div class="login-options">
        <el-checkbox v-model="remember">记住登录状态</el-checkbox>
      </div>
      <el-button type="primary" @click="handleLogin" :loading="loading" size="large" class="login-btn">
        登录
      </el-button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import http from '../utils/request.js'

const route = useRoute()
const router = useRouter()
const token = ref('')
const loading = ref(false)
const tokenInput = ref(null)
const remember = ref(false)

onMounted(async () => {
  tokenInput.value?.focus()
  try {
    const res = await http.get('/api/auth/check')
    if (res.data.data?.authenticated) {
      router.push(route.query.redirect || '/files')
    }
  } catch {
    // 未认证，留在登录页
  }
})

async function handleLogin() {
  if (!token.value.trim()) {
    ElMessage.warning('请输入访问令牌')
    return
  }

  loading.value = true
  try {
    await http.post('/api/auth/login', { token: token.value.trim(), remember: remember.value })
    router.push(route.query.redirect || '/files')
  } catch {
    // 拦截器已处理错误消息
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  width: 100%;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 380px;
  max-width: calc(100vw - 32px);
  padding: 40px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
}

.login-title {
  text-align: center;
  font-size: 28px;
  color: #303133;
  margin-bottom: 8px;
}

.login-subtitle {
  text-align: center;
  font-size: 14px;
  color: #909399;
  margin-bottom: 28px;
}

.login-options {
  display: flex;
  align-items: center;
  margin-top: 12px;
}

.login-btn {
  width: 100%;
  margin-top: 16px;
}

html.dark .login-page {
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
}

html.dark .login-card {
  background: #1d1e1f;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

html.dark .login-title {
  color: #e5eaf3;
}

html.dark .login-subtitle {
  color: #a3a6ad;
}
</style>
