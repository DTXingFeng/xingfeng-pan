<template>
  <div class="trash-page">
    <div class="navbar">
      <div class="navbar-left">
        <el-button text @click="$router.push('/files')" :icon="Back">返回文件列表</el-button>
        <span class="navbar-title">回收站</span>
      </div>
      <div class="navbar-right">
        <el-button type="danger" :disabled="files.length === 0" @click="handleEmptyTrash" :icon="Delete">
          清空回收站
        </el-button>
      </div>
    </div>

    <div class="trash-content" v-loading="loading">
      <el-empty v-if="!loading && files.length === 0" description="回收站为空" />

      <el-table v-else :data="files" style="width: 100%" @selection-change="onSelectionChange">
        <el-table-column type="selection" width="40" />
        <el-table-column prop="name" label="文件名" min-width="300">
          <template #default="{ row }">
            <div class="list-file-name">
              <el-icon v-if="row.is_dir" color="#f0c040"><Folder /></el-icon>
              <el-icon v-else-if="isImage(row.ext)" color="#67c23a"><Picture /></el-icon>
              <el-icon v-else-if="isVideo(row.ext)" color="#409eff"><VideoCamera /></el-icon>
              <el-icon v-else color="#909399"><Document /></el-icon>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{ row }">
            {{ row.is_dir ? '-' : formatSize(row.size) }}
          </template>
        </el-table-column>
        <el-table-column prop="deleted_at" label="删除时间" width="180" sortable />
        <el-table-column label="剩余天数" width="140">
          <template #default="{ row }">
            <span v-if="row.deleted_at" :style="{ color: daysLeft(row.deleted_at) <= 3 ? '#f56c6c' : '#909399' }">
              {{ daysLeftText(row.deleted_at) }}
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" text size="small" @click="handleRestore(row)">恢复</el-button>
            <el-button type="danger" text size="small" @click="handleDelete(row)">彻底删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 批量操作栏 -->
      <div v-if="selected.length > 0" class="batch-bar">
        <span>已选 {{ selected.length }} 项</span>
        <el-button type="primary" size="small" @click="batchRestore">批量恢复</el-button>
        <el-button type="danger" size="small" @click="batchDelete">批量删除</el-button>
      </div>

      <!-- 分页 -->
      <div v-if="total > pageSize" class="trash-pagination">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next"
          @current-change="loadTrash"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import http from '../utils/request.js'
import { Back, Folder, Picture, VideoCamera, Document, Delete } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const files = ref([])
const loading = ref(false)
const selected = ref([])
const page = ref(1)
const pageSize = 50
const total = ref(0)

const imageExts = new Set(['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tiff', 'tif'])
const videoExts = new Set(['mp4', 'avi', 'mkv', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'mpeg', 'mpg', '3gp'])

function isImage(ext) { return imageExts.has((ext || '').toLowerCase()) }
function isVideo(ext) { return videoExts.has((ext || '').toLowerCase()) }

function onSelectionChange(rows) { selected.value = rows }

async function loadTrash() {
  loading.value = true
  try {
    const resp = await http.get('/api/trash', { params: { page: page.value, page_size: pageSize } })
    const data = resp.data.data || {}
    files.value = data.files || []
    total.value = data.total || 0
  } catch {
    // 拦截器已处理
  } finally {
    loading.value = false
  }
}

async function handleRestore(file) {
  try {
    await http.post('/api/trash/restore', { uuid: file.uuid })
    ElMessage.success('已恢复')
    loadTrash()
  } catch {
    // 拦截器已处理
  }
}

async function handleDelete(file) {
  try {
    await ElMessageBox.confirm(`确定彻底删除「${file.name}」？不可恢复！`, '彻底删除', { type: 'warning' })
    await http.post('/api/trash/delete', { uuid: file.uuid })
    ElMessage.success('已彻底删除')
    loadTrash()
  } catch { /* 用户取消 */ }
}

async function handleEmptyTrash() {
  try {
    await ElMessageBox.confirm(`确定清空回收站？${files.value.length} 个项目将被永久删除！`, '清空回收站', { type: 'error', confirmButtonText: '确定清空' })
    const resp = await http.post('/api/trash/empty')
    const count = resp.data.data?.count || 0
    ElMessage.success(`回收站已清空，共删除 ${count} 个文件`)
    loadTrash()
  } catch { /* 用户取消 */ }
}

async function batchRestore() {
  for (const f of selected.value) {
    await http.post('/api/trash/restore', { uuid: f.uuid })
  }
  ElMessage.success(`已恢复 ${selected.value.length} 个项目`)
  loadTrash()
}

async function batchDelete() {
  try {
    await ElMessageBox.confirm(`确定彻底删除选中的 ${selected.value.length} 个项目？`, '批量删除', { type: 'warning' })
    for (const f of selected.value) {
      await http.post('/api/trash/delete', { uuid: f.uuid })
    }
    ElMessage.success(`已删除 ${selected.value.length} 个项目`)
    loadTrash()
  } catch { /* 用户取消 */ }
}

function formatSize(size) {
  if (!size && size !== 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let fsize = size, unit = 0
  while (fsize >= 1024 && unit < units.length - 1) { fsize /= 1024; unit++ }
  return fsize.toFixed(2) + ' ' + units[unit]
}

const TRASH_RETENTION_DAYS = 30

function daysLeft(deletedAt) {
  const deleted = new Date(deletedAt)
  const expire = deleted.getTime() + TRASH_RETENTION_DAYS * 86400000
  return Math.max(0, Math.ceil((expire - Date.now()) / 86400000))
}

function daysLeftText(deletedAt) {
  const days = daysLeft(deletedAt)
  if (days <= 0) return '即将清理'
  if (days <= 1) return '1天后自动清理'
  return days + '天后自动清理'
}

onMounted(loadTrash)
</script>

<style scoped>
.trash-page {
  width: 100%; max-width: 100vw; min-height: 100vh;
  display: flex; flex-direction: column;
  background: #f5f7fa; overflow-x: hidden;
}
.navbar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 20px; background: #fff;
  border-bottom: 1px solid #e4e7ed; box-shadow: 0 1px 4px rgba(0,0,0,0.04);
}
.navbar-left { display: flex; align-items: center; gap: 16px; }
.navbar-right { display: flex; align-items: center; gap: 8px; }
.navbar-title { font-size: 16px; font-weight: 600; color: #303133; }
.trash-content { flex: 1; padding: 20px 24px; max-width: 100%; overflow-x: auto; }
.list-file-name { display: flex; align-items: center; gap: 8px; }
.batch-bar {
  margin-top: 12px; padding: 10px 16px; background: #ecf5ff;
  border-radius: 6px; display: flex; align-items: center; gap: 12px;
}
.trash-pagination {
  margin-top: 16px; display: flex; justify-content: center;
}

html.dark .trash-page { background: #141414; }
html.dark .navbar { background: #1d1d1d; border-color: #3a3a3a; }
html.dark .navbar-title { color: #e5eaf3; }
html.dark .batch-bar { background: #1a2233; color: #e5eaf3; }

@media (max-width: 768px) {
  .navbar { padding: 10px 12px; }
  .navbar-left { gap: 8px; }
  .trash-content { padding: 10px; }
}
</style>
