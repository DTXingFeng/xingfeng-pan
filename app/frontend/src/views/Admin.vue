<template>
  <div class="admin-page">
    <div class="navbar">
      <div class="navbar-left">
        <el-button text @click="$router.push('/files')" :icon="Back">返回网盘</el-button>
        <span class="navbar-title">管理面板</span>
      </div>
    </div>

    <div class="admin-content">
      <!-- 仪表盘概览 -->
      <div class="section">
        <h3 class="section-title">系统概览</h3>
        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-label">总文件</div>
            <div class="stat-value">{{ dashboard.total_files?.toLocaleString() }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">总目录</div>
            <div class="stat-value">{{ dashboard.total_dirs?.toLocaleString() }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">总大小</div>
            <div class="stat-value">{{ formatSize(dashboard.total_size) }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">回收站</div>
            <div class="stat-value">{{ dashboard.trash_count }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">运行时间</div>
            <div class="stat-value stat-sm">{{ dashboard.uptime }}</div>
          </div>
        </div>
      </div>

      <!-- 磁盘使用 -->
      <div class="section">
        <h3 class="section-title">磁盘使用</h3>
        <div class="disk-bar-wrap">
          <el-progress
            :percentage="diskPercent"
            :stroke-width="20"
            :color="diskColor"
            :format="() => formatSize(dashboard.disk_used) + ' / ' + formatSize(dashboard.disk_total)"
          />
        </div>
      </div>

      <!-- 缩略图管理 -->
      <div class="section">
        <h3 class="section-title">缩略图状态</h3>
        <div class="stats-grid">
          <div class="stat-card stat-success">
            <div class="stat-label">已生成</div>
            <div class="stat-value">{{ dashboard.thumb_success?.toLocaleString() }}</div>
          </div>
          <div class="stat-card stat-fail">
            <div class="stat-label">生成失败</div>
            <div class="stat-value">{{ dashboard.thumb_failed?.toLocaleString() }}</div>
          </div>
          <div class="stat-card stat-pending">
            <div class="stat-label">待生成</div>
            <div class="stat-value">{{ dashboard.thumb_pending?.toLocaleString() }}</div>
          </div>
          <div class="stat-card stat-queue">
            <div class="stat-label">队列中</div>
            <div class="stat-value">{{ dashboard.thumb_queue }}</div>
          </div>
        </div>
        <div class="section-actions">
          <el-button type="warning" @click="retryAllFailed" :disabled="dashboard.thumb_failed === 0">
            重试全部失败 ({{ dashboard.thumb_failed }})
          </el-button>
          <el-button @click="refreshAll">刷新数据</el-button>
        </div>
      </div>

      <!-- 失败文件列表 -->
      <div class="section" v-if="dashboard.thumb_failed > 0">
        <h3 class="section-title">失败文件列表</h3>
        <el-table :data="failedFiles" v-loading="failedLoading" style="width: 100%" size="small">
          <el-table-column prop="name" label="文件名" min-width="300">
            <template #default="{ row }">
              <div class="file-name-cell">
                <el-icon v-if="isImage(row.ext)" color="#67c23a"><Picture /></el-icon>
                <el-icon v-else-if="isVideo(row.ext)" color="#409eff"><VideoCamera /></el-icon>
                <el-icon v-else color="#909399"><Document /></el-icon>
                <span>{{ row.name }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="size" label="大小" width="120">
            <template #default="{ row }">{{ formatSize(row.size) }}</template>
          </el-table-column>
          <el-table-column prop="rel_path" label="路径" min-width="200" show-overflow-tooltip />
          <el-table-column prop="created_at" label="创建时间" width="170" />
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="{ row }">
              <el-button type="primary" text size="small" @click="retryOne(row)">重试</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="pagination-wrap" v-if="failedTotal > failedPageSize">
          <el-pagination
            v-model:current-page="failedPage"
            :page-size="failedPageSize"
            :total="failedTotal"
            layout="prev, pager, next"
            @current-change="loadFailedFiles"
          />
        </div>
      </div>

      <!-- 系统配置 -->
      <div class="section">
        <h3 class="section-title">系统配置</h3>
        <el-form label-width="120px" style="max-width: 500px">
          <el-form-item label="端口">
            <el-input :model-value="String(config.port)" disabled />
          </el-form-item>
          <el-form-item label="挂载目录">
            <el-input :model-value="config.mount_path" disabled />
          </el-form-item>
          <el-form-item label="WebDAV 用户">
            <el-input v-model="configForm.webdav_user" placeholder="WebDAV 用户名" />
          </el-form-item>
          <el-form-item label="WebDAV 密码">
            <el-input v-model="configForm.webdav_pass" type="password" placeholder="留空不修改" show-password />
          </el-form-item>
          <el-form-item label="FFmpeg 路径">
            <el-input v-model="configForm.ffmpeg_path" placeholder="ffmpeg" />
          </el-form-item>
          <el-form-item label="认证 Token">
            <el-input v-model="configForm.auth_token" :placeholder="config.has_auth ? '留空不修改，输入 CLEAR 清除' : '留空不启用认证'" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="saveConfig" :loading="configSaving">保存配置</el-button>
            <span style="margin-left: 12px; font-size: 12px; color: #e6a23c;">注意：修改配置后需重启服务才能生效</span>
          </el-form-item>
        </el-form>
      </div>

      <!-- WebDAV 连接信息 -->
      <div class="section">
        <h3 class="section-title">WebDAV 连接信息</h3>
        <div class="webdav-info">
          <div class="detail-row">
            <span class="detail-label">连接地址</span>
            <span class="detail-value">{{ webdavURL }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">用户名</span>
            <span class="detail-value">{{ config.webdav_user || '未设置' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">密码状态</span>
            <span class="detail-value" :style="{ color: config.has_webdav_pass ? '#67c23a' : '#f56c6c' }">
              {{ config.has_webdav_pass ? '已设置' : '未设置' }}
            </span>
          </div>
        </div>
      </div>

      <!-- 运行日志 -->
      <div class="section">
        <h3 class="section-title">
          运行日志
          <el-button size="small" style="float: right" @click="loadLogs">刷新</el-button>
        </h3>
        <div v-if="logLines.length > 0" class="log-viewer">
          <pre>{{ logLines.join('\n') }}</pre>
        </div>
        <el-empty v-else description="暂无日志" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import http from '../utils/request.js'
import { Back, Picture, VideoCamera, Document } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const dashboard = ref({})
const failedFiles = ref([])
const failedLoading = ref(false)
const failedPage = ref(1)
const failedPageSize = ref(50)
const failedTotal = ref(0)

const config = ref({})
const configForm = reactive({
  webdav_user: '',
  webdav_pass: '',
  ffmpeg_path: '',
  auth_token: '',
})
const configSaving = ref(false)

const logLines = ref([])

async function loadLogs() {
  try {
    const resp = await http.get('/api/admin/logs', { params: { lines: 100 } })
    logLines.value = resp.data.data?.lines || []
  } catch {
    logLines.value = ['加载日志失败']
  }
}

const imageExts = new Set(['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tiff', 'tif'])
const videoExts = new Set(['mp4', 'avi', 'mkv', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'mpeg', 'mpg', '3gp'])

function isImage(ext) { return imageExts.has((ext || '').toLowerCase()) }
function isVideo(ext) { return videoExts.has((ext || '').toLowerCase()) }

function formatSize(size) {
  if (!size && size !== 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let fsize = size
  let unit = 0
  while (fsize >= 1024 && unit < units.length - 1) {
    fsize /= 1024
    unit++
  }
  return fsize.toFixed(2) + ' ' + units[unit]
}

const diskPercent = computed(() => {
  if (!dashboard.value.disk_total) return 0
  return Math.round((dashboard.value.disk_used / dashboard.value.disk_total) * 100)
})

const diskColor = computed(() => {
  const p = diskPercent.value
  if (p >= 90) return '#f56c6c'
  if (p >= 70) return '#e6a23c'
  return '#67c23a'
})

const webdavURL = computed(() => {
  return window.location.origin + '/dav'
})

async function loadDashboard() {
  try {
    const resp = await http.get('/api/admin/dashboard')
    dashboard.value = resp.data.data || {}
  } catch (e) {
    console.error('加载仪表盘失败', e)
  }
}

async function loadFailedFiles() {
  failedLoading.value = true
  try {
    const resp = await http.get('/api/admin/thumb/failed', {
      params: { page: failedPage.value, page_size: failedPageSize.value },
    })
    const data = resp.data.data || {}
    failedFiles.value = data.items || []
    failedTotal.value = data.total || 0
  } catch (e) {
    console.error('加载失败文件列表失败', e)
  } finally {
    failedLoading.value = false
  }
}

async function loadConfig() {
  try {
    const resp = await http.get('/api/admin/config')
    config.value = resp.data.data || {}
    configForm.webdav_user = config.value.webdav_user || ''
    configForm.ffmpeg_path = config.value.ffmpeg_path || ''
    configForm.auth_token = ''
    configForm.webdav_pass = ''
  } catch (e) {
    console.error('加载配置失败', e)
  }
}

async function retryAllFailed() {
  try {
    await ElMessageBox.confirm(
      `确定要重试全部 ${dashboard.value.thumb_failed} 个失败的缩略图吗？`,
      '批量重试',
      { type: 'warning' }
    )
    const resp = await http.post('/api/admin/thumb/retry-all')
    const count = resp.data.data?.count || 0
    ElMessage.success(`已提交 ${count} 个重试任务`)
    refreshAll()
  } catch {
    // 用户取消
  }
}

async function retryOne(row) {
  try {
    await http.post('/api/admin/thumb/retry', { uuid: row.uuid })
    ElMessage.success('已提交重试任务')
    refreshAll()
  } catch {
    // 错误已由拦截器处理
  }
}

async function saveConfig() {
  configSaving.value = true
  try {
    await http.post('/api/admin/config', configForm)
    ElMessage.success('配置已保存')
    await loadConfig()
    ElMessageBox.alert('配置已保存。注意：修改端口、WebDAV密码、挂载目录等需要重启服务才能生效。', '提示', { confirmButtonText: '知道了', type: 'warning' })
  } catch {
    // 错误已由拦截器处理
  } finally {
    configSaving.value = false
  }
}

function refreshAll() {
  loadDashboard()
  loadFailedFiles()
}

onMounted(() => {
  loadDashboard()
  loadFailedFiles()
  loadConfig()
  loadLogs()
})
</script>

<style scoped>
.admin-page {
  width: 100%;
  max-width: 100vw;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
  overflow-x: hidden;
}

.navbar {
  display: flex;
  align-items: center;
  padding: 12px 20px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
}

.navbar-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.navbar-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.admin-content {
  flex: 1;
  padding: 20px 24px;
  max-width: 100%;
}

.section {
  background: #fff;
  border-radius: 8px;
  padding: 20px 24px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px 0;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
}

.stat-card {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 14px 16px;
  text-align: center;
}

.stat-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 6px;
}

.stat-value {
  font-size: 22px;
  font-weight: 700;
  color: #303133;
}

.stat-value.stat-sm {
  font-size: 16px;
  font-weight: 600;
}

.stat-success { border-left: 3px solid #67c23a; }
.stat-success .stat-value { color: #67c23a; }
.stat-fail { border-left: 3px solid #f56c6c; }
.stat-fail .stat-value { color: #f56c6c; }
.stat-pending { border-left: 3px solid #e6a23c; }
.stat-pending .stat-value { color: #e6a23c; }
.stat-queue { border-left: 3px solid #409eff; }
.stat-queue .stat-value { color: #409eff; }

.disk-bar-wrap {
  max-width: 600px;
}

.section-actions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pagination-wrap {
  margin-top: 16px;
  display: flex;
  justify-content: center;
}

.webdav-info .detail-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid #ebeef5;
}
.webdav-info .detail-label {
  font-size: 13px;
  color: #909399;
  flex-shrink: 0;
  margin-right: 16px;
}
.webdav-info .detail-value {
  font-size: 13px;
  color: #303133;
  word-break: break-all;
  font-family: monospace;
}

.log-viewer {
  background: #1a1a2e;
  border-radius: 6px;
  padding: 12px 16px;
  max-height: 400px;
  overflow: auto;
}
.log-viewer pre {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: #c9d1d9;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  white-space: pre-wrap;
  word-break: break-all;
}
html.dark .log-viewer {
  background: #0d1117;
}

@media (max-width: 768px) {
  .navbar { padding: 10px 12px; }
  .navbar-left { gap: 8px; }
  .admin-content { padding: 10px; }
  .section { padding: 14px; }
  .stats-grid { grid-template-columns: repeat(2, 1fr); }
}

html.dark .admin-page { background: #141414; }
html.dark .section { background: #1d1d1d; }
html.dark .navbar { background: #1d1d1d; border-color: #3a3a3a; }
html.dark .navbar-title { color: #e5eaf3; }
html.dark .section-title { color: #e5eaf3; }
html.dark .stat-card { background: #262626; }
html.dark .stat-label { color: #73767a; }
html.dark .stat-value { color: #e5eaf3; }
html.dark .stat-success .stat-value { color: #67c23a; }
html.dark .stat-fail .stat-value { color: #f56c6c; }
html.dark .stat-pending .stat-value { color: #e6a23c; }
html.dark .stat-queue .stat-value { color: #409eff; }
html.dark .webdav-info .detail-row { border-color: #3a3a3a; }
html.dark .webdav-info .detail-label { color: #73767a; }
html.dark .webdav-info .detail-value { color: #e5eaf3; }
</style>
