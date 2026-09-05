<template>
  <div class="file-manager">
    <!-- 顶部导航栏 -->
    <div class="navbar">
      <div class="navbar-left">
        <!-- 搜索框 -->
        <el-input
          v-model="searchKeyword"
          class="search-input"
          placeholder="搜索文件..."
          :prefix-icon="Search"
          clearable
          @keyup.enter="doSearch"
          @input="onSearchInput"
          @clear="clearSearch"
        />
        <el-button text @click="goHome" :icon="HomeFilled">
          <span class="btn-text">首页</span>
        </el-button>
        <el-breadcrumb separator="/">
          <el-breadcrumb-item>
            <span class="breadcrumb-link" @click="goHome" @dragover.prevent @drop.prevent="onDropMove('')">全部文件</span>
          </el-breadcrumb-item>
          <el-breadcrumb-item v-for="(item, idx) in breadcrumbs" :key="idx">
            <span class="breadcrumb-link" @click="navigateTo(item.path)" @dragover.prevent @drop.prevent="onDropMove(item.path)">{{ item.name }}</span>
          </el-breadcrumb-item>
        </el-breadcrumb>
      </div>
      <div class="navbar-right">
        <!-- 粘贴按钮 -->
        <el-button
          v-if="clipboard.files.length > 0 && !batchMode"
          @click="pasteFromClipboard"
          :icon="clipboard.type === 'copy' ? CopyDocument : Edit"
        ><span class="btn-text">{{ clipboard.type === 'copy' ? '粘贴' : '粘贴(剪切)' }}</span></el-button>
        <!-- 批量操作 -->
        <el-button
          v-if="!batchMode"
          @click="enterBatchMode"
          :icon="Check"
        ><span class="btn-text">批量操作</span></el-button>
        <template v-else>
          <el-button @click="toggleSelectAll" :icon="isAllSelected ? Close : Check">
            <span class="btn-text">{{ isAllSelected ? '取消全选' : '全选' }}</span>
          </el-button>
          <el-button
            type="danger"
            :disabled="selectedFiles.size === 0"
            @click="batchDelete"
            :icon="Delete"
          ><span class="btn-text">删除 ({{ selectedFiles.size }})</span></el-button>
          <el-button
            type="primary"
            :disabled="selectedFiles.size === 0"
            @click="batchDownload"
            :icon="Download"
          ><span class="btn-text">下载 ({{ selectedFiles.size }})</span></el-button>
          <el-button
            :disabled="selectedFiles.size === 0"
            @click="batchRegenThumb"
          ><span class="btn-text">重新生成缩略图</span></el-button>
          <el-button @click="exitBatchMode"><span class="btn-text">取消</span></el-button>
        </template>
        <template v-if="!batchMode">
          <el-button @click="showMkdirDialog" :icon="FolderAdd"><span class="btn-text">新建</span></el-button>
          <el-button @click="triggerFileInput" :icon="UploadFilled" type="primary"><span class="btn-text">上传</span></el-button>
          <input
            ref="fileInputRef"
            type="file"
            multiple
            style="display: none"
            @change="handleFileSelect"
          />
          <!-- 文件夹上传 -->
          <el-button @click="triggerFolderInput" title="上传文件夹"><span class="btn-text">📁</span></el-button>
          <input
            ref="folderInputRef"
            type="file"
            webkitdirectory
            directory
            multiple
            style="display: none"
            @change="handleFolderSelect"
          />
          <!-- 桌面端：完整按钮 -->
          <span class="desktop-only">
            <el-button @click="$router.push('/trash')" :icon="Delete"><span class="btn-text">回收站</span></el-button>
            <el-button @click="$router.push('/admin')"><span class="btn-text">管理</span></el-button>
            <el-dropdown @command="handleSort" trigger="click">
              <el-button :icon="SortUp"><span class="btn-text">排序</span></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="name_asc" :class="{ 'is-active': sortBy === 'name' && sortOrder === 'asc' }">名称↑</el-dropdown-item>
                  <el-dropdown-item command="name_desc" :class="{ 'is-active': sortBy === 'name' && sortOrder === 'desc' }">名称↓</el-dropdown-item>
                  <el-dropdown-item command="size_asc" :class="{ 'is-active': sortBy === 'size' && sortOrder === 'asc' }">大小↑</el-dropdown-item>
                  <el-dropdown-item command="size_desc" :class="{ 'is-active': sortBy === 'size' && sortOrder === 'desc' }">大小↓</el-dropdown-item>
                  <el-dropdown-item command="time_asc" :class="{ 'is-active': sortBy === 'created_at' && sortOrder === 'asc' }">时间↑</el-dropdown-item>
                  <el-dropdown-item command="time_desc" :class="{ 'is-active': sortBy === 'created_at' && sortOrder === 'desc' }">时间↓</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-dropdown @command="handleFilterType" trigger="click">
              <el-button><span class="btn-text">{{ filterTypeLabel }}</span></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="all">全部</el-dropdown-item>
                  <el-dropdown-item command="image">图片</el-dropdown-item>
                  <el-dropdown-item command="video">视频</el-dropdown-item>
                  <el-dropdown-item command="audio">音频</el-dropdown-item>
                  <el-dropdown-item command="document">文档</el-dropdown-item>
                  <el-dropdown-item command="other">其他</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button-group>
              <el-button :type="viewMode === 'grid' ? 'primary' : ''" @click="viewMode = 'grid'" :icon="Grid" />
              <el-button :type="viewMode === 'list' ? 'primary' : ''" @click="viewMode = 'list'" :icon="List" />
            </el-button-group>
          </span>
          <!-- 移动端：更多菜单 -->
          <el-dropdown class="mobile-only" trigger="click">
            <el-button :icon="MoreFilled" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="$router.push('/trash')"><el-icon><Delete /></el-icon> 回收站</el-dropdown-item>
                <el-dropdown-item @click="$router.push('/admin')"><el-icon><Setting /></el-icon> 管理</el-dropdown-item>
                <el-dropdown-item divided>
                  <el-dropdown @command="handleSort" trigger="hover" size="small">
                    排序 <el-icon><ArrowRight /></el-icon>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item command="name_asc">名称↑</el-dropdown-item>
                        <el-dropdown-item command="name_desc">名称↓</el-dropdown-item>
                        <el-dropdown-item command="size_asc">大小↑</el-dropdown-item>
                        <el-dropdown-item command="size_desc">大小↓</el-dropdown-item>
                        <el-dropdown-item command="time_asc">时间↑</el-dropdown-item>
                        <el-dropdown-item command="time_desc">时间↓</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </el-dropdown-item>
                <el-dropdown-item>
                  <el-dropdown @command="handleFilterType" trigger="hover" size="small">
                    筛选 <el-icon><ArrowRight /></el-icon>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item command="all">全部</el-dropdown-item>
                        <el-dropdown-item command="image">图片</el-dropdown-item>
                        <el-dropdown-item command="video">视频</el-dropdown-item>
                        <el-dropdown-item command="audio">音频</el-dropdown-item>
                        <el-dropdown-item command="document">文档</el-dropdown-item>
                        <el-dropdown-item command="other">其他</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </el-dropdown-item>
                <el-dropdown-item @click="viewMode = viewMode === 'grid' ? 'list' : 'grid'">
                  {{ viewMode === 'grid' ? '列表视图' : '网格视图' }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <!-- 暗色模式切换 -->
          <el-button @click="toggleDarkMode" :icon="isDark ? Moon : Sunny" circle />
          <el-button @click="showShortcutHelp" :icon="QuestionFilled" circle title="键盘快捷键" />
          <!-- 存储统计 -->
          <span class="stats-text desktop-only" v-if="statsInfo">
            文件: {{ statsInfo.total_files }} | 大小: {{ formatSize(statsInfo.total_size) }}
            <span v-if="statsInfo.thumb_pending > 0" style="color: #e6a23c; margin-left: 4px">| 缩略图: {{ statsInfo.thumb_pending }}个待生成</span>
          </span>
        </template>
      </div>
    </div>

    <!-- 文件列表区域（支持拖拽） -->
    <div
      class="file-content"
      @dragover.prevent="onDragOver"
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
      :class="{ 'drag-active': isDragging }"
    >
      <!-- 拖拽提示遮罩 -->
      <div v-if="isDragging" class="drag-overlay">
        <el-icon :size="64" color="#409eff"><UploadFilled /></el-icon>
        <p>松开鼠标上传文件到当前目录</p>
      </div>

    <!-- 上传进度悬浮通知 -->
    <transition name="upload-slide">
      <div v-if="uploadingFiles.length > 0" class="upload-toast">
        <div class="upload-toast-header">
          <span>上传中 ({{ uploadingFiles.filter(f => f.status === 'success').length }}/{{ uploadingFiles.length }})</span>
          <el-button text size="small" @click="uploadingFiles = []">关闭</el-button>
        </div>
        <div class="upload-toast-body">
          <div v-for="uf in uploadingFiles" :key="uf.name" class="upload-toast-item">
            <span class="upload-name">{{ uf.name }}</span>
            <el-progress :percentage="uf.status === 'skipped' ? 100 : uf.progress" :status="uf.status === 'skipped' ? 'warning' : uf.status" :stroke-width="4" style="flex: 1" />
            <span v-if="uf.status === 'skipped'" class="upload-skipped-tag">重复跳过</span>
            <span v-if="uf.speed" class="upload-speed">{{ uf.speed }}</span>
          </div>
        </div>
      </div>
    </transition>

      <!-- 搜索结果提示 -->
      <div v-if="isSearching" class="search-hint">
        <span>搜索结果：找到 {{ filteredFiles.length }} 个文件</span>
        <el-button type="primary" text @click="clearSearch">清除搜索</el-button>
      </div>

      <!-- 搜索过滤器 -->
      <div v-if="isSearching" class="search-filters">
        <el-radio-group v-model="searchFilterType" size="small" @change="doSearch">
          <el-radio-button value="all">全部</el-radio-button>
          <el-radio-button value="image">图片</el-radio-button>
          <el-radio-button value="video">视频</el-radio-button>
          <el-radio-button value="audio">音频</el-radio-button>
          <el-radio-button value="document">文档</el-radio-button>
          <el-radio-button value="other">其他</el-radio-button>
        </el-radio-group>
      </div>

      <!-- 最近访问 -->
      <div v-if="!isSearching && currentPath === '' && recentVisits.length > 0" class="recent-visits">
        <span class="recent-label">最近访问：</span>
        <el-tag
          v-for="(item, idx) in recentVisits"
          :key="idx"
          closable
          size="small"
          @click="navigateTo(item.path)"
          @close="removeRecentVisit(idx)"
        >{{ item.name || '根目录' }}</el-tag>
      </div>

      <!-- 返回上级 -->
      <div v-if="parentPath !== null && parentPath !== undefined && currentPath !== '' && !isSearching" class="go-back" @click="navigateTo(parentPath)" @dragover.prevent @drop.prevent="onDropMove(parentPath)">
        <el-icon><Back /></el-icon>
        <span>返回上级目录</span>
      </div>

      <!-- 空目录 -->
      <el-empty v-if="!loading && filteredFiles.length === 0 && uploadingFiles.length === 0" :description="isSearching ? '没有找到匹配的文件' : '目录为空，拖拽文件到此处上传'" />

      <!-- 骨架屏加载 -->
      <div v-if="loading && filteredFiles.length === 0" class="file-grid skeleton-grid">
        <div v-for="i in 12" :key="'s'+i" class="file-card skeleton-card">
          <div class="skeleton-thumb" />
          <div class="skeleton-line" />
          <div class="skeleton-line short" />
        </div>
      </div>

      <!-- 卡片模式 -->
      <div v-if="viewMode === 'grid'" class="file-grid">
        <div
          v-for="file in filteredFiles"
          :key="file.uuid"
          class="file-card"
          :class="{ 'file-card-selected': selectedFiles.has(file.uuid) }"
          draggable="true"
          @dragstart="onFileDragStart($event, file)"
          @click="onCardClick(file)"
          @contextmenu.prevent="onCardContextMenu($event, file)"
          @touchstart="onTouchStart($event, file)"
          @touchend="onTouchEnd"
          @touchmove="onTouchEnd"
        >
          <!-- 批量模式复选框 -->
          <div v-if="batchMode" class="file-card-checkbox" @click.stop="toggleSelect(file)">
            <el-checkbox :model-value="selectedFiles.has(file.uuid)" />
          </div>
          <div class="file-card-thumb">
            <el-icon v-if="file.is_dir" :size="64" color="#f0c040"><Folder /></el-icon>
            <img
              v-else-if="file.has_thumbnail && isImage(file.ext)"
              :src="`/api/thumb?uuid=${file.uuid}`"
              alt=""
              draggable="false"
              loading="lazy"
              @error="e => e.target.style.display='none'"
              @contextmenu.prevent
            />
            <img
              v-else-if="file.has_thumbnail && isVideo(file.ext)"
              :src="`/api/thumb?uuid=${file.uuid}`"
              alt=""
              draggable="false"
              loading="lazy"
              @error="e => e.target.style.display='none'"
              @contextmenu.prevent
            />
            <el-icon v-else-if="isVideo(file.ext)" :size="64" color="#409eff"><VideoCamera /></el-icon>
            <el-icon v-else-if="isImage(file.ext)" :size="64" color="#67c23a"><Picture /></el-icon>
            <el-icon v-else-if="isAudio(file.ext)" :size="64" color="#e6a23c"><Headset /></el-icon>
            <el-icon v-else :size="64" color="#909399"><Document /></el-icon>
          </div>
          <div class="file-card-name" :title="file.name">{{ file.name }}</div>
          <div class="file-card-sub" v-if="isSearching && file.rel_path" :title="file.rel_path">{{ file.rel_path }}</div>
          <div class="file-card-info">
            <span v-if="!file.is_dir">{{ formatSize(file.size) }}</span>
            <span v-else>文件夹</span>
          </div>
        </div>
      </div>

      <!-- 列表模式 -->
      <el-table
        v-if="viewMode === 'list'"
        :data="filteredFiles"
        @row-click="onTableRowClick"
        @row-contextmenu="onTableRowContextMenu"
        style="width: 100%"
        :row-class-name="tableRowClassName"
      >
        <el-table-column v-if="batchMode" width="50">
          <template #default="{ row }">
            <el-checkbox :model-value="selectedFiles.has(row.uuid)" @click.stop @change="toggleSelect(row)" />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="文件名" min-width="300">
          <template #default="{ row }">
            <div class="list-file-name">
              <el-icon v-if="row.is_dir" color="#f0c040"><Folder /></el-icon>
              <el-icon v-else-if="isImage(row.ext)" color="#67c23a"><Picture /></el-icon>
              <el-icon v-else-if="isVideo(row.ext)" color="#409eff"><VideoCamera /></el-icon>
              <el-icon v-else-if="isAudio(row.ext)" color="#e6a23c"><Headset /></el-icon>
              <el-icon v-else color="#909399"><Document /></el-icon>
              <span>{{ row.name }}</span>
            </div>
            <div v-if="isSearching && row.rel_path" class="list-file-path">{{ row.rel_path }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{ row }">
            {{ row.is_dir ? '-' : formatSize(row.size) }}
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180" />
        <el-table-column v-if="!batchMode" label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button type="danger" text size="small" @click.stop="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div v-if="totalCount > pageSize" class="pagination-bar">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="totalCount"
          :page-sizes="[50, 100, 200, 500]"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <!-- 图片预览 -->
    <el-image-viewer
      v-if="imageViewerVisible"
      :url-list="imageViewerList"
      :initial-index="imageViewerIndex"
      @close="imageViewerVisible = false"
    />

    <!-- 视频播放 Dialog -->
    <el-dialog
      v-model="videoDialogVisible"
      :title="videoTitle"
      width="70%"
      destroy-on-close
      @close="onVideoDialogClose"
    >
      <div class="video-wrapper">
        <div v-if="videoNeedsTranscode && !videoReady && !videoError" class="video-transcode-tip">
          <el-icon :size="40" color="#409EFF" class="transcode-spin"><Loading /></el-icon>
          <p style="margin: 12px 0 4px; font-size: 15px; color: #606266">
            {{ videoLoadingText }}
          </p>
          <p style="margin: 0; font-size: 13px; color: #909399">
            {{ videoCodecInfo || '视频格式需要服务端处理后才能播放' }}
          </p>
        </div>
        <video
          v-if="videoDialogVisible && !videoError"
          ref="videoPlayer"
          :src="videoSrc"
          controls
          autoplay
          disablepictureinpicture="false"
          class="video-player"
          :style="{ display: (videoNeedsTranscode && !videoReady) ? 'none' : 'block' }"
          @contextmenu.prevent
          @error="onVideoError"
          @loadeddata="onVideoReady"
        />
        <div v-if="videoError" class="video-error-tip">
          <el-icon :size="48" color="#909399"><VideoCamera /></el-icon>
          <p style="margin: 12px 0 4px; font-size: 15px; color: #606266">
            该视频无法在浏览器直接播放
          </p>
          <p style="margin: 0 0 16px; font-size: 13px; color: #909399">
            {{ videoCodecInfo ? videoCodecInfo + '，' : '' }}编码不兼容或转码失败。可尝试强制转码、
            复制直链用 PotPlayer/VLC 打开，或手机端继续用 WebDAV 播放器观看
          </p>
          <div style="display: flex; gap: 12px; justify-content: center; flex-wrap: wrap">
            <el-button type="success" @click="openInPotPlayer(videoCurrentFile)">
              用 PotPlayer 打开
            </el-button>
            <el-button v-if="videoMode !== 'transcode'" type="warning" @click="forceTranscodePlay">
              强制转码播放
            </el-button>
            <el-button @click="copyVideoLink">复制直链</el-button>
            <el-button type="primary" @click="downloadCurrentVideo">
              <el-icon style="margin-right: 4px"><Download /></el-icon>下载视频
            </el-button>
          </div>
        </div>
        <div class="video-speed-bar" v-if="!videoError && (!videoNeedsTranscode || videoReady)">
          <span>倍速：</span>
          <el-button-group size="small">
            <el-button v-for="s in [0.5, 0.75, 1, 1.25, 1.5, 2]" :key="s"
              :type="videoSpeed === s ? 'primary' : ''"
              @click="setVideoSpeed(s)">{{ s }}x</el-button>
          </el-button-group>
          <el-button
            size="small"
            :type="volumeBalance ? 'primary' : ''"
            @click="toggleVolumeBalance"
            style="margin-left: 12px"
          >音量平衡{{ volumeBalance ? ' ✓' : '' }}</el-button>
          <el-button size="small" @click="openInPotPlayer(videoCurrentFile)" style="margin-left: 12px">
            PotPlayer 播放
          </el-button>
          <el-button size="small" :icon="FullScreen" @click="toggleVideoFullscreen" style="margin-left: auto">全屏</el-button>
        </div>
      </div>
    </el-dialog>

    <!-- PotPlayer 协议帮助 Dialog -->
    <el-dialog v-model="potHelpVisible" title="使用 PotPlayer 打开" width="520px">
      <div style="font-size: 14px; line-height: 1.9; color: #606266">
        <p style="margin-top: 0">点击「PotPlayer 播放」会通过 <code>potplayer://</code> 协议唤起本地 PotPlayer 直接播放原始视频流，<b>不经过服务器转码</b>，HEVC/AC3/DTS 等任何格式都能秒开，也不占用服务器 CPU。</p>
        <p style="color: #e6a23c">看起来没有反应？说明这台电脑还没装协议支持（PotPlayer 升级有时也会重置协议注册），按下面两步配置一次即可：</p>
        <el-form label-width="110px" label-position="left">
          <el-form-item label="PotPlayer 路径">
            <el-input v-model="potPathInput" placeholder="C:\Program Files\PotPlayer\PotPlayerMini64.exe" />
          </el-form-item>
        </el-form>
        <el-button type="primary" @click="downloadPotReg">下载注册文件 (.reg)</el-button>
        <p style="margin-bottom: 0; font-size: 13px; color: #909399">
          1. 确认上面的路径与本机 PotPlayer 安装位置一致（可在 PotPlayer 安装目录找到 PotPlayerMini64.exe）<br>
          2. 下载后双击导入注册表，回到网页再点一次「PotPlayer 播放」即可，浏览器首次会询问是否允许打开
        </p>
      </div>
    </el-dialog>

    <!-- 音频播放 Dialog -->
    <el-dialog
      v-model="audioDialogVisible"
      :title="audioTitle"
      width="500px"
      destroy-on-close
      @close="audioSrc = ''"
    >
      <div class="audio-player-wrapper">
        <audio
          v-if="audioDialogVisible"
          :src="audioSrc"
          controls
          autoplay
          style="width: 100%"
        />
      </div>
    </el-dialog>

    <!-- 文件预览 Dialog（文本/PDF） -->
    <el-dialog
      v-model="previewDialogVisible"
      :title="previewTitle"
      width="80%"
      destroy-on-close
      @close="previewSrc = ''"
      class="preview-dialog"
    >
      <div class="preview-content" v-loading="previewLoading">
        <iframe v-if="previewIsPdf" :src="previewSrc" class="preview-iframe" />
        <pre v-else-if="previewText" class="preview-text">{{ previewText }}</pre>
        <el-empty v-else description="不支持预览此文件类型" />
      </div>
    </el-dialog>

    <!-- 新建文件夹 Dialog -->
    <el-dialog v-model="mkdirDialogVisible" title="新建文件夹" width="400px">
      <el-input v-model="mkdirName" placeholder="请输入文件夹名称" @keyup.enter="doMkdir" />
      <template #footer>
        <el-button @click="mkdirDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doMkdir" :loading="mkdirLoading">创建</el-button>
      </template>
    </el-dialog>

    <!-- 重命名 Dialog -->
    <el-dialog v-model="renameDialogVisible" title="重命名" width="400px">
      <el-input v-model="renameNewName" placeholder="请输入新名称" @keyup.enter="doRename" />
      <template #footer>
        <el-button @click="renameDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doRename" :loading="renameLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 移动/复制到 Dialog -->
    <el-dialog v-model="moveDialogVisible" :title="moveDialogTitle" width="450px">
      <div class="move-dialog-content">
        <p>目标目录：</p>
        <div class="dir-browser">
          <div class="dir-current" @click="browseTo('')">
            <el-icon><HomeFilled /></el-icon> 根目录
          </div>
          <div v-if="moveBrowsePath" class="dir-breadcrumb">
            <span v-for="(seg, i) in moveBrowsePath.split('/')" :key="i"
              class="dir-seg" @click="browseTo(moveBrowsePath.split('/').slice(0, i+1).join('/'))">
              / {{ seg }}
            </span>
          </div>
          <div class="dir-list" v-loading="moveBrowseLoading">
            <div v-for="d in moveBrowseDirs" :key="d.rel_path" class="dir-item"
              :class="{ active: moveTargetPath === d.rel_path }"
              @click="moveTargetPath = d.rel_path"
              @dblclick="browseTo(d.rel_path)">
              <el-icon color="#f0c040"><Folder /></el-icon> {{ d.name }}
            </div>
            <div v-if="!moveBrowseLoading && moveBrowseDirs.length === 0" class="dir-empty">无子目录</div>
          </div>
        </div>
        <el-input v-model="moveTargetPath" placeholder="或手动输入路径" size="small" style="margin-top: 8px">
          <template #prefix><el-icon><FolderOpened /></el-icon></template>
        </el-input>
      </div>
      <template #footer>
        <el-button @click="moveDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doMove" :loading="moveLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 文件详情抽屉 -->
    <el-drawer
      v-model="detailDrawerVisible"
      title="文件详情"
      direction="rtl"
      size="380px"
    >
      <div v-if="detailInfo" class="file-detail">
        <div class="detail-row">
          <span class="detail-label">文件名</span>
          <span class="detail-value">{{ detailInfo.name }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">路径</span>
          <span class="detail-value">{{ detailInfo.rel_path }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">类型</span>
          <span class="detail-value">{{ detailInfo.is_dir ? '文件夹' : '文件' }}</span>
        </div>
        <div class="detail-row" v-if="!detailInfo.is_dir">
          <span class="detail-label">大小</span>
          <span class="detail-value">{{ formatSize(detailInfo.size) }}</span>
        </div>
        <div class="detail-row" v-if="!detailInfo.is_dir">
          <span class="detail-label">扩展名</span>
          <span class="detail-value">{{ detailInfo.ext || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">创建时间</span>
          <span class="detail-value">{{ detailInfo.created_at }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">缩略图</span>
          <span class="detail-value">{{ detailInfo.has_thumbnail ? '有' : '无' }}</span>
        </div>
        <div class="detail-row" v-if="detailInfo.is_dir">
          <span class="detail-label">子项数量</span>
          <span class="detail-value">{{ detailInfo.child_count }}</span>
        </div>
        <div class="detail-footer">
          <el-button @click="detailDrawerVisible = false">关闭</el-button>
        </div>
      </div>
      <div v-else class="file-detail">
        <p>加载中...</p>
      </div>
    </el-drawer>

    <!-- 右键菜单 -->
    <div
      v-if="contextMenuVisible"
      class="context-menu"
      :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
      @click.stop
    >
      <div class="context-menu-item" @click="handleContextOpen">打开</div>
      <div v-if="!contextFile?.is_dir && isVideo(contextFile?.ext)" class="context-menu-item" @click="handleContextPotPlayer">用 PotPlayer 播放</div>
      <div v-if="!contextFile?.is_dir" class="context-menu-item" @click="handleContextDownload">下载</div>
      <div v-if="contextFile?.is_dir" class="context-menu-item" @click="handleContextZipDownload">打包下载</div>
      <div class="context-menu-divider" />
      <div class="context-menu-item" @click="handleContextCopyTo">复制到</div>
      <div class="context-menu-item" @click="handleContextMoveTo">移动到</div>
      <div class="context-menu-divider" />
      <div class="context-menu-item" @click="handleContextCopy">复制</div>
      <div class="context-menu-item" @click="handleContextCut">剪切</div>
      <div class="context-menu-divider" />
      <div class="context-menu-item" @click="handleContextRename">重命名</div>
      <div class="context-menu-item" @click="handleContextDetail">文件详情</div>
      <div v-if="!contextFile?.is_dir" class="context-menu-item" @click="handleContextRegenThumb">重新生成缩略图</div>
      <div v-if="!contextFile?.is_dir && isVideo(contextFile?.ext)" class="context-menu-item" @click="handleContextVideoRepair">修复视频卡顿</div>
      <div class="context-menu-divider" />
      <div class="context-menu-item danger" @click="handleContextDelete">删除</div>
    </div>

    <!-- 底部固定批量操作栏 -->
    <div v-if="batchMode" class="batch-bottom-bar">
      <span class="batch-info">{{ selectedFiles.size > 0 ? '已选 ' + selectedFiles.size + ' 项' : '请选择文件' }}</span>
      <el-button size="small" @click="toggleSelectAll" :icon="isAllSelected ? Close : Check">{{ isAllSelected ? '取消全选' : '全选' }}</el-button>
      <el-button v-if="selectedFiles.size > 0" type="danger" size="small" @click="batchDelete" :icon="Delete">删除</el-button>
      <el-button v-if="selectedFiles.size > 0" type="primary" size="small" @click="batchDownload" :icon="Download">下载</el-button>
      <el-button v-if="selectedFiles.size > 0" size="small" @click="batchRegenThumb">重新生成缩略图</el-button>
      <el-button size="small" @click="exitBatchMode">取消</el-button>
    </div>

    <!-- 迷你音频播放器 -->
    <transition name="audio-slide">
      <div v-if="audioSrc && !audioDialogVisible" class="mini-audio-bar">
        <span class="mini-audio-title">{{ audioTitle }}</span>
        <audio :src="audioSrc" controls autoplay class="mini-audio-player" />
        <el-button text :icon="Close" @click="audioSrc = ''" />
      </div>
    </transition>

    <!-- 键盘快捷键帮助 -->
    <el-dialog v-model="shortcutHelpVisible" title="键盘快捷键" width="480px">
      <el-table :data="shortcutList" size="small" :show-header="false">
        <el-table-column prop="key" label="快捷键" width="180" />
        <el-table-column prop="desc" label="说明" />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import http from '../utils/request.js'
import {
  HomeFilled, Delete, Grid, List, Back, Folder, Picture,
  VideoCamera, Document, UploadFilled, FolderAdd, Check, Close, Download,
  Headset, Sunny, Moon, Search, CopyDocument, Edit, FolderOpened,
  MoreFilled, Setting, ArrowRight, SortUp, FullScreen, QuestionFilled, Loading,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()

const files = ref([])
const loading = ref(false)
const currentPath = ref('')
const parentPath = ref('')

// 滚动位置记忆栈：进入子目录时 push，返回时 pop 恢复
const scrollPositionStack = []
// 导航方向：'forward' 进入子目录 | 'back' 返回上级 | 'reset' 其他跳转
let navDirection = 'reset'
const viewMode = ref(localStorage.getItem('viewMode') || 'grid')

// 批量操作
const batchMode = ref(false)
const selectedFiles = ref(new Set())

// 拖拽上传
const isDragging = ref(false)
const uploadingFiles = ref([])

// 文件选择
const fileInputRef = ref(null)
const folderInputRef = ref(null)

// 图片预览
const imageViewerVisible = ref(false)
const imageViewerList = ref([])
const imageViewerIndex = ref(0)

// 视频播放
const videoDialogVisible = ref(false)
const videoTitle = ref('')
const videoSrc = ref('')
const videoSpeed = ref(1)
const videoError = ref(false)
const videoCurrentFile = ref(null)
// 播放模式：pending 探测中 / direct 直连 / copy 转封装 / audio 转音轨 / transcode 实时转码
const videoMode = ref('direct')
const videoNeedsTranscode = computed(() => videoMode.value !== 'direct')
const videoReady = ref(false)
// 探测到的编码信息（用于界面提示）
const videoCodecInfo = ref('')
const videoLoadingText = computed(() => ({
  pending: '正在检测视频编码…',
  copy: '正在转封装中…',
  audio: '音轨不兼容，正在转换音频…',
  transcode: '正在实时转码（H.264）…首次缓冲可能较慢',
}[videoMode.value] || '正在处理中…'))

// 音量平衡
const volumeBalance = ref(false)
let audioCtx = null
let gainNode = null
let sourceNode = null
let analyserNode = null
let balanceRafId = null

// 用户视频偏好持久化
const VIDEO_PREFS_KEY = 'pan_video_prefs'
function loadVideoPrefs() {
  try {
    const raw = localStorage.getItem(VIDEO_PREFS_KEY)
    if (raw) return JSON.parse(raw)
  } catch (_) {}
  return {}
}
function saveVideoPref(key, value) {
  const prefs = loadVideoPrefs()
  prefs[key] = value
  try { localStorage.setItem(VIDEO_PREFS_KEY, JSON.stringify(prefs)) } catch (_) {}
}
function getVideoPref(key, defaultValue) {
  const prefs = loadVideoPrefs()
  return prefs[key] !== undefined ? prefs[key] : defaultValue
}

// 初始化：从 localStorage 恢复音量平衡开关状态
volumeBalance.value = getVideoPref('volumeBalance', false)

// 音频播放
const audioDialogVisible = ref(false)
const audioTitle = ref('')
const audioSrc = ref('')

// 文件预览
const previewDialogVisible = ref(false)
const previewTitle = ref('')
const previewSrc = ref('')
const previewText = ref('')
const previewIsPdf = ref(false)
const previewLoading = ref(false)

// 新建文件夹
const mkdirDialogVisible = ref(false)
const mkdirName = ref('')
const mkdirLoading = ref(false)

// 重命名
const renameDialogVisible = ref(false)
const renameFile = ref(null)
const renameNewName = ref('')
const renameLoading = ref(false)

// 移动/复制到
const moveDialogVisible = ref(false)
const moveDialogTitle = ref('')
const moveMode = ref('') // 'copy' 或 'move'
const moveTargetFile = ref(null)
const moveTargetPath = ref('')
const moveLoading = ref(false)
const moveBrowsePath = ref('')
const moveBrowseDirs = ref([])
const moveBrowseLoading = ref(false)

// 文件详情
const detailDrawerVisible = ref(false)
const detailInfo = ref(null)

// 右键菜单
const contextMenuVisible = ref(false)
const contextMenuX = ref(0)
const contextMenuY = ref(0)
const contextFile = ref(null)

// 搜索
const searchKeyword = ref('')
const isSearching = ref(false)
const savedFiles = ref([])

// 排序
const sortBy = ref(localStorage.getItem('sortBy') || 'name')
const sortOrder = ref(localStorage.getItem('sortOrder') || 'asc')
const currentPage = ref(1)
const pageSize = ref(100)
const totalCount = ref(0)

// 文件类型筛选
const filterType = ref(localStorage.getItem('filterType') || 'all')

// 剪贴板（sessionStorage 持久化，刷新不丢失）
const savedClipboard = (() => { try { const v = sessionStorage.getItem('clipboard'); return v ? JSON.parse(v) : null } catch { return null } })()
const clipboard = reactive(savedClipboard || { type: '', files: [] })
watch(() => clipboard, (v) => { sessionStorage.setItem('clipboard', JSON.stringify({ type: v.type, files: v.files.map(f => ({ uuid: f.uuid, name: f.name })) })) }, { deep: true })
function clearClipboard() { clipboard.type = ''; clipboard.files = []; sessionStorage.removeItem('clipboard') }

// 暗色模式
const isDark = ref(false)

// 存储统计
const statsInfo = ref(null)

// 搜索过滤
const searchFilterType = ref('all')

// 最近访问
const recentVisits = ref(JSON.parse(localStorage.getItem('recentVisits') || '[]'))

// 键盘快捷键帮助
const shortcutHelpVisible = ref(false)
const shortcutList = [
  { key: 'Escape', desc: '退出批量模式 / 清除搜索' },
  { key: 'Delete / Backspace', desc: '批量模式下删除选中文件' },
  { key: 'F2', desc: '重命名单个选中文件' },
  { key: 'Ctrl + A', desc: '全选（进入批量模式）' },
  { key: 'Ctrl + C', desc: '复制选中文件到剪贴板' },
  { key: 'Ctrl + X', desc: '剪切选中文件到剪贴板' },
  { key: 'Ctrl + V', desc: '粘贴剪贴板中的文件' },
  { key: 'Enter', desc: '搜索框中确认搜索' },
  { key: '右键 / 长按', desc: '打开文件操作菜单' },
]

const imageExts = new Set(['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tiff', 'tif'])
const videoExts = new Set(['mp4', 'avi', 'mkv', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'mpeg', 'mpg', '3gp'])
const audioExts = new Set(['mp3', 'wav', 'flac', 'aac', 'ogg', 'wma', 'm4a'])
const docExts = new Set(['doc', 'docx', 'pdf', 'txt', 'xls', 'xlsx', 'ppt', 'pptx', 'md', 'csv', 'rtf', 'odt', 'ods', 'odp', 'pages', 'numbers', 'key'])
const textPreviewExts = new Set(['txt', 'md', 'csv', 'json', 'xml', 'html', 'css', 'js', 'ts', 'py', 'go', 'java', 'c', 'cpp', 'h', 'sh', 'bat', 'yaml', 'yml', 'ini', 'cfg', 'log', 'sql', 'conf'])
const pdfExts = new Set(['pdf'])
const officeExts = new Set(['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'ods', 'odp'])

function isImage(ext) { return imageExts.has((ext || '').toLowerCase()) }
function isVideo(ext) { return videoExts.has((ext || '').toLowerCase()) }
function isAudio(ext) { return audioExts.has((ext || '').toLowerCase()) }
function isDoc(ext) { return docExts.has((ext || '').toLowerCase()) }

// 文件类型已由后端过滤，直接返回列表
const filteredFiles = computed(() => {
  return files.value
})

const filterTypeLabel = computed(() => {
  const map = { all: '全部', image: '图片', video: '视频', audio: '音频', document: '文档', other: '其他' }
  return map[filterType.value] || '全部'
})

// 是否全选
const isAllSelected = computed(() => {
  return files.value.length > 0 && selectedFiles.value.size === files.value.length
})

// 面包屑：基于当前相对路径
const breadcrumbs = computed(() => {
  if (!currentPath.value) return []
  const parts = currentPath.value.split('/').filter(Boolean)
  const result = []
  let accumulated = ''
  for (const part of parts) {
    accumulated = accumulated ? accumulated + '/' + part : part
    result.push({ name: part, path: accumulated })
  }
  return result
})

// === 搜索（防抖） ===

let searchDebounceTimer = null

function onSearchInput() {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  if (!searchKeyword.value.trim()) {
    clearSearch()
    return
  }
  searchDebounceTimer = setTimeout(() => {
    doSearch()
  }, 300)
}

async function doSearch() {
  const keyword = searchKeyword.value.trim()
  if (!keyword) {
    clearSearch()
    return
  }
  loading.value = true
  try {
    const params = { keyword, path: currentPath.value || '', page: 1, page_size: 100 }
    if (searchFilterType.value !== 'all') {
      params.type = searchFilterType.value
    }
    const resp = await http.get('/api/search', { params })
    const data = resp.data.data
    savedFiles.value = files.value
    files.value = data.files || []
    totalCount.value = data.total || 0
    isSearching.value = true
    exitBatchMode()
  } catch {
    console.error('搜索失败')
  } finally {
    loading.value = false
  }
}

function clearSearch() {
  searchKeyword.value = ''
  isSearching.value = false
  if (savedFiles.value.length > 0) {
    files.value = savedFiles.value
    savedFiles.value = []
  } else {
    loadFiles(currentPath.value)
  }
}

// === 排序 ===

function handleSort(command) {
  const [sort, order] = command.split('_')
  sortBy.value = sort === 'time' ? 'created_at' : sort
  sortOrder.value = order
  loadFiles(currentPath.value)
}

// === 文件类型筛选 ===

function handleFilterType(type) {
  filterType.value = type
  loadFiles(currentPath.value)
}

// === 剪贴板 ===

function handleContextCopy() {
  if (!contextFile.value) return
  hideContextMenu()
  clipboard.type = 'copy'
  clipboard.files = [contextFile.value]
  ElMessage.success('已复制到剪贴板')
}

function handleContextCut() {
  if (!contextFile.value) return
  hideContextMenu()
  clipboard.type = 'cut'
  clipboard.files = [contextFile.value]
  ElMessage.success('已剪切到剪贴板')
}

async function pasteFromClipboard() {
  if (clipboard.files.length === 0) return
  const targetPath = currentPath.value || ''
  const api = clipboard.type === 'cut' ? '/api/move' : '/api/copy'
  let success = 0
  let fail = 0
  for (const file of clipboard.files) {
    try {
      await http.post(api, { uuid: file.uuid, target_path: targetPath })
      success++
    } catch {
      fail++
    }
  }
  if (fail === 0) {
    ElMessage.success(`成功${clipboard.type === 'cut' ? '移动' : '复制'} ${success} 个文件`)
  } else {
    ElMessage.warning(`${success} 个成功，${fail} 个失败`)
  }
  clearClipboard()
  loadFiles(currentPath.value)
}

// === 暗色模式 ===

function toggleDarkMode() {
  document.documentElement.classList.toggle('dark')
  isDark.value = document.documentElement.classList.contains('dark')
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initDarkMode() {
  const saved = localStorage.getItem('theme')
  if (saved === 'dark') {
    document.documentElement.classList.add('dark')
    isDark.value = true
  } else {
    isDark.value = false
  }
}

// === 存储统计 ===

async function loadStats() {
  try {
    const resp = await http.get('/api/stats')
    statsInfo.value = resp.data.data
  } catch {
    // 忽略
  }
}

// === 重命名 ===

function handleContextRename() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  renameFile.value = file
  renameNewName.value = file.name
  renameDialogVisible.value = true
}

async function doRename() {
  if (!renameFile.value || !renameNewName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  renameLoading.value = true
  try {
    await http.post('/api/rename', {
      uuid: renameFile.value.uuid,
      new_name: renameNewName.value.trim(),
    })
    ElMessage.success('重命名成功')
    renameDialogVisible.value = false
    loadFiles(currentPath.value)
  } catch {
    // 错误已由拦截器处理
  } finally {
    renameLoading.value = false
  }
}

// === 移动/复制到 ===

function handleContextCopyTo() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  moveMode.value = 'copy'
  moveTargetFile.value = file
  moveDialogTitle.value = '复制到'
  moveTargetPath.value = ''
  moveDialogVisible.value = true
  browseTo('')
}

function handleContextMoveTo() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  moveMode.value = 'move'
  moveTargetFile.value = file
  moveDialogTitle.value = '移动到'
  moveTargetPath.value = ''
  moveDialogVisible.value = true
  browseTo('')
}

async function browseTo(path) {
  moveBrowsePath.value = path
  moveTargetPath.value = path
  moveBrowseLoading.value = true
  try {
    const resp = await http.get('/api/list', { params: { path, page: 1, page_size: 500 } })
    const data = resp.data.data
    moveBrowseDirs.value = (data?.files || []).filter(f => f.is_dir)
  } catch {
    moveBrowseDirs.value = []
  } finally {
    moveBrowseLoading.value = false
  }
}

async function doMove() {
  if (!moveTargetFile.value) return
  moveLoading.value = true
  const api = moveMode.value === 'copy' ? '/api/copy' : '/api/move'
  try {
    await http.post(api, {
      uuid: moveTargetFile.value.uuid,
      target_path: moveTargetPath.value.trim(),
    })
    ElMessage.success(moveMode.value === 'copy' ? '复制成功' : '移动成功')
    moveDialogVisible.value = false
    loadFiles(currentPath.value)
  } catch {
    // 错误已由拦截器处理
  } finally {
    moveLoading.value = false
  }
}

// === 文件详情 ===

function handleContextDetail() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  showFileDetail(file.uuid)
}

async function showFileDetail(uuid) {
  detailInfo.value = null
  detailDrawerVisible.value = true
  try {
    const resp = await http.get('/api/file/info', { params: { uuid } })
    detailInfo.value = resp.data.data
  } catch {
    ElMessage.error('获取文件详情失败')
    detailDrawerVisible.value = false
  }
}

// === 打包下载 ===

function handleContextZipDownload() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  const link = document.createElement('a')
  link.href = `/api/download/zip?uuid=${file.uuid}`
  link.download = file.name + '.zip'
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// === 批量操作 ===

function enterBatchMode() {
  batchMode.value = true
  selectedFiles.value = new Set()
}

function exitBatchMode() {
  batchMode.value = false
  selectedFiles.value = new Set()
}

function toggleSelect(file) {
  const s = new Set(selectedFiles.value)
  if (s.has(file.uuid)) {
    s.delete(file.uuid)
  } else {
    s.add(file.uuid)
  }
  selectedFiles.value = s
}

function toggleSelectAll() {
  if (isAllSelected.value) {
    selectedFiles.value = new Set()
  } else {
    selectedFiles.value = new Set(files.value.map(f => f.uuid))
  }
}

async function batchDelete() {
  const count = selectedFiles.value.size
  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${count} 个文件吗？文件将移入回收站。`,
      '批量删除确认',
      { type: 'warning' }
    )
  } catch {
    return
  }

  const uuids = Array.from(selectedFiles.value)
  try {
    const resp = await http.post('/api/batch/delete', { uuids })
    const results = resp.data.data || []
    const failed = results.filter(r => !r.success)
    if (failed.length === 0) {
      ElMessage.success(`成功删除 ${count} 个文件`)
    } else {
      ElMessage.warning(`${count - failed.length} 个成功，${failed.length} 个失败`)
    }
    exitBatchMode()
    loadFiles(currentPath.value)
  } catch {
    // 错误已由拦截器处理
  }
}

function batchDownload() {
  const selected = files.value.filter(f => selectedFiles.value.has(f.uuid) && !f.is_dir)
  if (selected.length === 0) {
    ElMessage.warning('没有可下载的文件')
    return
  }
  // 逐个触发下载（浏览器限制并发，但会自动排队）
  for (const file of selected) {
    triggerDownload(file)
  }
  ElMessage.success(`开始下载 ${selected.length} 个文件`)
}

async function batchRegenThumb() {
  const selected = files.value.filter(f => selectedFiles.value.has(f.uuid) && !f.is_dir)
  if (selected.length === 0) {
    ElMessage.warning('没有可重新生成的文件')
    return
  }
  let count = 0
  for (const file of selected) {
    try {
      await http.post('/api/file/regen-thumb', { uuid: file.uuid })
      count++
    } catch { /* 拦截器已处理 */ }
  }
  ElMessage.success(`已提交 ${count} 个缩略图重新生成任务`)
}

// 卡片点击：批量模式下切换选中，普通模式下打开
function onCardClick(file) {
  if (batchMode.value) {
    toggleSelect(file)
  } else {
    handleClick(file)
  }
}

function onCardContextMenu(e, file) {
  if (batchMode.value) return
  showContextMenu(e, file)
}

// 移动端长按触发右键菜单
let longPressTimer = null
function onTouchStart(e, file) {
  longPressTimer = setTimeout(() => {
    const touch = e.touches[0]
    showContextMenu({ clientX: touch.clientX, clientY: touch.clientY, preventDefault() {} }, file)
  }, 500)
}
function onTouchEnd() {
  if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null }
}

// 列表行点击
function onTableRowClick(row) {
  if (batchMode.value) {
    toggleSelect(row)
  } else {
    handleClick(row)
  }
}

// 列表行右键菜单
function onTableRowContextMenu(row, column, event) {
  if (batchMode.value) return
  event.preventDefault()
  showContextMenu(event, row)
}

function tableRowClassName({ row }) {
  if (batchMode.value && selectedFiles.value.has(row.uuid)) {
    return 'row-selected'
  }
  return ''
}

// 加载文件列表
async function loadFiles(path, page) {
  loading.value = true
  try {
    const params = {}
    if (path) params.path = path
    params.sort = sortBy.value
    params.order = sortOrder.value
    params.page = page || currentPage.value
    params.page_size = pageSize.value
    if (filterType.value && filterType.value !== 'all') {
      params.type = filterType.value
    }
    const resp = await http.get('/api/list', { params })
    const data = resp.data.data
    files.value = data.files || []
    currentPath.value = data.path || ''
    parentPath.value = data.parent !== undefined ? data.parent : ''
    totalCount.value = data.total || 0
    currentPage.value = data.page || 1
  } catch (e) {
    console.error('加载文件列表失败', e)
  } finally {
    loading.value = false
  }
}

function handlePageChange(page) {
  if (document.activeElement) document.activeElement.blur()
  router.replace({ query: { ...route.query, page: page > 1 ? String(page) : undefined } })
  loadFiles(currentPath.value, page).then(() => {
    nextTick(() => scrollToTop())
  })
}

function handleSizeChange(size) {
  if (document.activeElement) document.activeElement.blur()
  pageSize.value = size
  currentPage.value = 1
  router.replace({ query: { ...route.query, page: undefined } })
  loadFiles(currentPath.value, 1).then(() => {
    nextTick(() => scrollToTop())
  })
}

function getScrollTop() {
  return window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop || 0
}

function scrollToTop() {
  // 同时重置所有可能的滚动容器
  window.scrollTo({ top: 0, behavior: 'instant' })
  document.documentElement.scrollTop = 0
  document.body.scrollTop = 0
  // 再用 rAF 确保 DOM 更新后也滚到顶部
  requestAnimationFrame(() => {
    window.scrollTo({ top: 0, behavior: 'instant' })
    document.documentElement.scrollTop = 0
    document.body.scrollTop = 0
  })
}

function scrollToPosition(top) {
  window.scrollTo({ top, behavior: 'instant' })
  document.documentElement.scrollTop = top
  document.body.scrollTop = top
  requestAnimationFrame(() => {
    window.scrollTo({ top, behavior: 'instant' })
    document.documentElement.scrollTop = top
    document.body.scrollTop = top
  })
}

// 判断导航方向并管理滚动位置栈
function prepareNavigation(targetPath) {
  const current = currentPath.value || ''
  const target = targetPath || ''

  const currentParts = current ? current.split('/') : []
  const targetParts = target ? target.split('/') : []
  const currentDepth = currentParts.length
  const targetDepth = targetParts.length

  // 判断是否为父子关系：较短路径是较长路径的前缀
  const isDescendant = targetDepth > currentDepth &&
    (current === '' || target.startsWith(current + '/'))
  const isAncestor = currentDepth > targetDepth &&
    (target === '' || current.startsWith(target + '/'))

  if (isDescendant) {
    // 进入子目录：保存当前滚动位置与页码
    navDirection = 'forward'
    scrollPositionStack.push({ path: current, scrollTop: getScrollTop(), page: currentPage.value })
  } else if (isAncestor) {
    // 返回上级目录：计算需要 pop 的层级数
    navDirection = 'back'
    const levelDiff = currentDepth - targetDepth
    // 多弹出的层级直接丢弃，只保留目标层的位置
    for (let i = 1; i < levelDiff; i++) {
      scrollPositionStack.pop()
    }
  } else {
    // 同级跳转或不相关跳转：清空栈
    navDirection = 'reset'
    scrollPositionStack.length = 0
  }
}

// 导航到指定相对路径
function navigateTo(path) {
  if (isSearching.value) clearSearch()
  prepareNavigation(path)
  saveRecentVisit(path)
  const target = path || ''
  if (navDirection === 'reset' && target === (currentPath.value || '')) {
    // 点击当前目录面包屑：刷新列表但保持页码（不走路由，避免重复导航被丢弃）
    loadFiles(target, currentPage.value).then(() => {
      nextTick(() => scrollToTop())
    })
    return
  }
  const query = {}
  if (target) query.path = target
  if (navDirection === 'back') {
    // 返回上级：恢复离开该目录时所在的页码
    const saved = scrollPositionStack.length ? scrollPositionStack[scrollPositionStack.length - 1] : null
    if (saved && saved.path === target && saved.page > 1) {
      query.page = String(saved.page)
    }
  }
  router.push({ path: '/files', query })
}

function saveRecentVisit(path) {
  const name = path ? path.split('/').pop() || '根目录' : '根目录'
  const visits = recentVisits.value.filter(v => v.path !== path)
  visits.unshift({ name, path: path || '' })
  if (visits.length > 10) visits.pop()
  recentVisits.value = visits
  localStorage.setItem('recentVisits', JSON.stringify(visits))
}

function removeRecentVisit(idx) {
  recentVisits.value.splice(idx, 1)
  localStorage.setItem('recentVisits', JSON.stringify(recentVisits.value))
}

function showShortcutHelp() {
  shortcutHelpVisible.value = true
}

// 拖拽移动
const dragFile = ref(null)

function onFileDragStart(e, file) {
  if (batchMode.value) return
  dragFile.value = file
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData('text/plain', file.name)
}

async function onDropMove(targetPath) {
  const file = dragFile.value
  if (!file) return
  if (file.rel_path === targetPath || (file.is_dir && targetPath.startsWith(file.rel_path))) {
    ElMessage.warning('不能移动到自身或子目录')
    return
  }
  try {
    await http.post('/api/move', { uuid: file.uuid, target_path: targetPath })
    ElMessage.success(`已移动到 ${targetPath || '根目录'}`)
    dragFile.value = null
    loadFiles(currentPath.value)
  } catch {
    // 拦截器已处理
  }
}

function goHome() {
  if (isSearching.value) clearSearch()
  navDirection = 'reset'
  scrollPositionStack.length = 0
  router.push('/files')
}

// 文件点击事件
const nativeVideoExts = new Set(['mp4', 'webm', 'ogg', 'mov', 'm4v'])

// —— 浏览器解码能力判断（配合 /api/probe 的编码探测结果）——
let hevcPlayableCache = null
function browserHEVC() {
  if (hevcPlayableCache === null) {
    try {
      const v = document.createElement('video')
      hevcPlayableCache = !!(v.canPlayType && v.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"'))
    } catch (_) {
      hevcPlayableCache = false
    }
  }
  return hevcPlayableCache
}
// 视频编码是否可由浏览器直接解码
function VIDEO_OK(codec) {
  if (['h264', 'vp8', 'vp9', 'av1', 'theora'].includes(codec)) return true
  if (codec === 'hevc') return browserHEVC()
  return false
}
// 音频编码在 mp4 容器内是否可由浏览器直接解码
function AUDIO_OK(codec) {
  return ['aac', 'mp3', 'flac', 'opus', 'vorbis'].includes(codec)
}

function setVideoSpeed(speed) {
  videoSpeed.value = speed
  const player = document.querySelector('.video-player')
  if (player) player.playbackRate = speed
}

function toggleVideoFullscreen() {
  const player = document.querySelector('.video-player')
  if (player) {
    if (document.fullscreenElement) {
      document.exitFullscreen()
    } else {
      player.requestFullscreen()
    }
  }
}

function onVideoReady() {
  videoReady.value = true
  const player = document.querySelector('.video-player')
  if (player) {
    // 恢复用户保存的音量
    const savedVolume = getVideoPref('volume', 1)
    player.volume = savedVolume
    // 监听音量变化，实时保存到 localStorage
    player.addEventListener('volumechange', () => {
      saveVideoPref('volume', player.volume)
    })
    // 建立 Web Audio 管线（始终连接，保证音频通路不断）
    initAudioPipeline(player)
    // 根据用户偏好决定是否启动音量平衡循环
    if (volumeBalance.value) {
      startBalanceLoop()
    }
  }
}

function onVideoDialogClose() {
  videoSrc.value = ''
  videoReady.value = false
  videoMode.value = 'direct'
  videoCodecInfo.value = ''
  cleanupVolumeBalance()
}

function onVideoError() {
  // 探测中或尚未设置源时忽略（如空 src 产生的伪错误）
  if (videoMode.value === 'pending' || !videoSrc.value) return
  // 逐级降级：direct → copy → audio → transcode → 报错
  const ladder = { direct: 'copy', copy: 'audio', audio: 'transcode' }
  const next = ladder[videoMode.value]
  if (next && videoCurrentFile.value) {
    applyVideoMode(next)
    return
  }
  videoError.value = true
}

// 按模式设置播放源
function applyVideoMode(mode) {
  videoMode.value = mode
  videoReady.value = false
  videoSrc.value = mode === 'direct'
    ? `/api/stream?uuid=${videoCurrentFile.value.uuid}`
    : `/api/transcode?uuid=${videoCurrentFile.value.uuid}&mode=${mode}`
}

// 打开视频：先用 ffprobe 探测编码选择播放策略，失败时靠降级链兜底
async function openVideo(file) {
  videoTitle.value = file.name
  videoCurrentFile.value = file
  videoError.value = false
  videoReady.value = false
  videoSpeed.value = 1
  videoCodecInfo.value = ''
  videoMode.value = 'pending'
  videoSrc.value = ''
  videoDialogVisible.value = true

  const ext = (file.ext || '').toLowerCase()
  const containerNative = nativeVideoExts.has(ext)
  // 无探测结果时的保守默认：native 扩展名直连，其余转封装
  let mode = containerNative ? 'direct' : 'copy'

  try {
    const resp = await http.get(`/api/probe?uuid=${file.uuid}`, { timeout: 8000 })
    const info = resp.data && resp.data.data
    if (info && info.available && info.video_codec) {
      const v = info.video_codec
      const a = info.audio_codec || ''
      videoCodecInfo.value = `视频编码 ${v.toUpperCase()}${a ? ` · 音频 ${a.toUpperCase()}` : ''}`
      const videoOK = VIDEO_OK(v)
      const audioOK = AUDIO_OK(a)
      if (containerNative && videoOK && audioOK) {
        mode = 'direct'
      } else if (videoOK && !audioOK) {
        mode = 'audio'      // 视频可播、音轨不兼容：只转音频，CPU 开销极低
      } else if (videoOK) {
        mode = 'copy'       // 编码兼容、容器不兼容：仅转封装
      } else {
        mode = 'transcode'  // HEVC 等浏览器不支持的编码：实时转码
      }
    }
  } catch (_) {
    // 探测失败（无 ffprobe / 超时）：按扩展名默认策略，交给降级链兜底
  }

  // 弹窗已被关闭或已切到别的文件时，丢弃过期的探测结果
  if (!videoDialogVisible.value || videoCurrentFile.value?.uuid !== file.uuid) return
  applyVideoMode(mode)
}

function forceTranscodePlay() {
  if (!videoCurrentFile.value) return
  videoError.value = false
  applyVideoMode('transcode')
}

// —— PotPlayer 本地播放（potplayer:// 自定义协议，零服务器转码）——
const potHelpVisible = ref(false)
const potPathInput = ref('C:\\Program Files\\PotPlayer\\PotPlayerMini64.exe')
let cachedPlayToken

async function getPlayToken() {
  if (cachedPlayToken !== undefined) return cachedPlayToken
  try {
    const resp = await http.get('/api/local-play-token')
    cachedPlayToken = (resp.data && resp.data.data && resp.data.data.token) || ''
  } catch (_) {
    cachedPlayToken = ''
  }
  return cachedPlayToken
}

async function openInPotPlayer(file) {
  if (!file) return
  potHelpVisible.value = false
  const token = await getPlayToken()
  let url = window.location.origin + `/api/stream?uuid=${file.uuid}`
  if (token) url += `&token=${encodeURIComponent(token)}`
  // 通过失焦判断协议是否已注册：注册成功会唤起 PotPlayer 使页面失焦
  let launched = false
  const onBlur = () => { launched = true }
  window.addEventListener('blur', onBlur)
  // 标准双斜杠格式 potplayer://http://...，PotPlayer 原生协议处理器可直接解析；
  // 旧版单斜杠 potplayer:/ 形式只有配合本站生成的 cmd 包装器才能用，
  // PotPlayer 升级时安装器会用原生处理器覆盖注册表导致播放失败
  window.location.href = 'potplayer://' + url
  setTimeout(() => {
    window.removeEventListener('blur', onBlur)
    if (!launched) potHelpVisible.value = true
  }, 1600)
}

// 生成注册 potplayer:// 协议的 .reg 文件（写入 HKCU，无需管理员）
// 仅在 PotPlayer 未自带协议注册时兜底（如绿色版）；命令行直调方式与
// PotPlayer 官方安装器写入的注册表一致，不会被格式解析问题卡住
function downloadPotReg() {
  const potPath = (potPathInput.value || '').trim() || 'C:\\Program Files\\PotPlayer\\PotPlayerMini64.exe'
  const inner = `"${potPath}" "%1"`
  const escaped = inner.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
  const lines = [
    'Windows Registry Editor Version 5.00',
    '',
    '[HKEY_CURRENT_USER\\Software\\Classes\\potplayer]',
    '@="URL:PotPlayer"',
    '"URL Protocol"=""',
    '',
    '[HKEY_CURRENT_USER\\Software\\Classes\\potplayer\\shell\\open\\command]',
    `@="${escaped}"`,
    '',
  ]
  const text = lines.join('\r\n')
  // .reg 标准编码为 UTF-16LE + BOM
  const bytes = new Uint8Array(2 + text.length * 2)
  bytes[0] = 0xFF
  bytes[1] = 0xFE
  const dv = new DataView(bytes.buffer)
  for (let i = 0; i < text.length; i++) {
    dv.setUint16(2 + i * 2, text.charCodeAt(i), true)
  }
  const blob = new Blob([bytes], { type: 'application/octet-stream' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'open-with-potplayer.reg'
  a.click()
  URL.revokeObjectURL(a.href)
  ElMessage.success('已下载，双击导入注册表后重试')
}

async function copyVideoLink() {
  if (!videoCurrentFile.value) return
  const token = await getPlayToken()
  let url = window.location.origin + `/api/stream?uuid=${videoCurrentFile.value.uuid}`
  if (token) url += `&token=${encodeURIComponent(token)}`
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(url).then(() => {
      ElMessage.success('直链已复制，可用 PotPlayer / VLC 等播放器打开')
    }).catch(() => {
      ElMessage.warning(url)
    })
  } else {
    ElMessage.warning(url)
  }
}

function downloadCurrentVideo() {
  if (videoCurrentFile.value) {
    triggerDownload(videoCurrentFile.value)
    videoDialogVisible.value = false
  }
}

// 目标 RMS 音量（0~1 范围，0.06 约等于舒适听感）
const TARGET_RMS = 0.06
// 增益限制范围
const MIN_GAIN = 0.2
const MAX_GAIN = 5.0
// 长期 RMS 采样窗口大小（约 3 秒 @60fps）
const RMS_WINDOW_SIZE = 180
// 增益平滑系数（越小越稳定，0.005 约需 10 秒收敛）
const GAIN_SMOOTH = 0.005

// 稳定增益值
let stableGain = 1.0
// 滑动窗口收集 RMS 样本
let rmsBuffer = []
let rmsSum = 0

// 初始化 Web Audio 管线：source → analyser → gain → destination
// 管线始终连接，关闭平衡时 gain=1 等于透传
function initAudioPipeline(player) {
  if (sourceNode) return

  try {
    if (!audioCtx) {
      audioCtx = new (window.AudioContext || window.webkitAudioContext)()
    }
    if (audioCtx.state === 'suspended') {
      audioCtx.resume()
    }

    sourceNode = audioCtx.createMediaElementSource(player)
    analyserNode = audioCtx.createAnalyser()
    analyserNode.fftSize = 4096
    analyserNode.smoothingTimeConstant = 0.9

    gainNode = audioCtx.createGain()
    gainNode.gain.value = 1.0

    sourceNode.connect(analyserNode)
    analyserNode.connect(gainNode)
    gainNode.connect(audioCtx.destination)

    stableGain = 1.0
    rmsBuffer = []
    rmsSum = 0
  } catch (e) {
    console.warn('[音量平衡] Web Audio 管线初始化失败:', e)
  }
}

function toggleVolumeBalance() {
  if (volumeBalance.value) {
    stopBalanceLoop()
    if (gainNode) gainNode.gain.value = 1.0
    stableGain = 1.0
    rmsBuffer = []
    rmsSum = 0
    volumeBalance.value = false
  } else {
    volumeBalance.value = true
    if (analyserNode && gainNode) {
      startBalanceLoop()
    }
  }
  saveVideoPref('volumeBalance', volumeBalance.value)
}

function startBalanceLoop() {
  const bufferLength = analyserNode.fftSize
  const dataArray = new Float32Array(bufferLength)

  function loop() {
    if (!volumeBalance.value || !analyserNode || !gainNode) return

    analyserNode.getFloatTimeDomainData(dataArray)

    // 计算当前帧 RMS
    let sum = 0
    for (let i = 0; i < bufferLength; i++) {
      sum += dataArray[i] * dataArray[i]
    }
    const rms = Math.sqrt(sum / bufferLength)

    // 只在有声音时采样
    if (rms > 0.005) {
      rmsBuffer.push(rms)
      rmsSum += rms
      // 维持滑动窗口大小
      if (rmsBuffer.length > RMS_WINDOW_SIZE) {
        rmsSum -= rmsBuffer.shift()
      }
    }

    // 窗口积累足够样本后才开始调节
    if (rmsBuffer.length >= 30) {
      const avgRms = rmsSum / rmsBuffer.length
      if (avgRms > 0.005) {
        // 获取用户音量滑块值
        const player = document.querySelector('.video-player')
        const userVolume = player ? player.volume : 1
        // 基于长期均值计算目标增益
        const targetGain = (TARGET_RMS / avgRms) * userVolume
        const clamped = Math.max(MIN_GAIN, Math.min(MAX_GAIN, targetGain))
        // 极慢平滑，避免增益抖动
        stableGain = stableGain + (clamped - stableGain) * GAIN_SMOOTH
        gainNode.gain.value = stableGain
      }
    }

    balanceRafId = requestAnimationFrame(loop)
  }

  balanceRafId = requestAnimationFrame(loop)
}

function stopBalanceLoop() {
  if (balanceRafId) {
    cancelAnimationFrame(balanceRafId)
    balanceRafId = null
  }
}

// 关闭视频对话框时释放全部 Web Audio 资源
function cleanupVolumeBalance() {
  stopBalanceLoop()

  if (gainNode) {
    gainNode.gain.value = 1.0
  }

  if (sourceNode) {
    try { sourceNode.disconnect() } catch (_) {}
    sourceNode = null
  }
  if (analyserNode) {
    try { analyserNode.disconnect() } catch (_) {}
    analyserNode = null
  }
  if (gainNode) {
    try { gainNode.disconnect() } catch (_) {}
    gainNode = null
  }
  if (audioCtx) {
    audioCtx.close().catch(() => {})
    audioCtx = null
  }

  stableGain = 1.0
  rmsBuffer = []
  rmsSum = 0
}

function handleClick(file) {
  if (file.is_dir) {
    navigateTo(file.rel_path)
  } else if (isImage(file.ext)) {
    const images = files.value.filter((f) => !f.is_dir && isImage(f.ext))
    const idx = images.findIndex((f) => f.uuid === file.uuid)
    imageViewerList.value = images.map((f) => `/api/stream?uuid=${f.uuid}`)
    imageViewerIndex.value = idx >= 0 ? idx : 0
    imageViewerVisible.value = true
  } else if (isVideo(file.ext)) {
    openVideo(file)
  } else if (isAudio(file.ext)) {
    audioTitle.value = file.name
    audioSrc.value = `/api/stream?uuid=${file.uuid}`
    audioDialogVisible.value = false
  } else if (pdfExts.has((file.ext || '').toLowerCase())) {
    previewTitle.value = file.name
    previewSrc.value = `/api/stream?uuid=${file.uuid}`
    previewIsPdf.value = true
    previewText.value = ''
    previewLoading.value = false
    previewDialogVisible.value = true
  } else if (textPreviewExts.has((file.ext || '').toLowerCase())) {
    previewTitle.value = file.name
    previewIsPdf.value = false
    previewText.value = ''
    previewLoading.value = true
    previewDialogVisible.value = true
    http.get(`/api/stream?uuid=${file.uuid}`, { responseType: 'text' })
      .then(resp => {
        previewText.value = typeof resp.data === 'string' ? resp.data : JSON.stringify(resp.data, null, 2)
      })
      .catch(() => {
        previewText.value = '预览加载失败'
      })
      .finally(() => {
        previewLoading.value = false
      })
  } else if (officeExts.has((file.ext || '').toLowerCase())) {
    const streamURL = encodeURIComponent(window.location.origin + `/api/stream?uuid=${file.uuid}`)
    previewTitle.value = file.name
    previewSrc.value = `https://view.officeapps.live.com/op/embed.aspx?src=${streamURL}`
    previewIsPdf.value = false
    previewText.value = ''
    previewLoading.value = false
    previewDialogVisible.value = true
  } else {
    triggerDownload(file)
  }
}

function triggerDownload(file) {
  const link = document.createElement('a')
  link.href = `/api/download?uuid=${file.uuid}`
  link.download = file.name
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// === 上传功能 ===

function triggerFileInput() {
  fileInputRef.value?.click()
}

function triggerFolderInput() {
  folderInputRef.value?.click()
}

function handleFileSelect(e) {
  const fileList = e.target.files
  if (fileList && fileList.length > 0) {
    uploadFiles(Array.from(fileList))
    e.target.value = ''
  }
}

function handleFolderSelect(e) {
  const fileList = e.target.files
  if (!fileList || fileList.length === 0) return

  // 收集所有需要创建的目录和文件
  const dirSet = new Set()
  const fileEntries = []
  for (const file of fileList) {
    const relativePath = file.webkitRelativePath
    if (!relativePath) continue
    const parts = relativePath.split('/')
    // 收集所有目录层级
    for (let i = 1; i < parts.length; i++) {
      dirSet.add(parts.slice(0, i).join('/'))
    }
    fileEntries.push({ file, relativePath })
  }

  uploadFolderFiles(Array.from(dirSet).sort(), fileEntries)
  e.target.value = ''
}

async function uploadFolderFiles(dirs, fileEntries) {
  const basePath = currentPath.value || ''

  // 先创建所有目录
  for (const dir of dirs) {
    try {
      await http.post('/api/mkdir', { path: basePath, name: dir })
    } catch {
      // 目录可能已存在，忽略错误
    }
  }

  // 逐个上传文件
  const CONCURRENT = 2
  let queue = [...fileEntries]

  async function processNext() {
    while (queue.length > 0) {
      const entry = queue.shift()
      if (!entry) break

      const dir = entry.relativePath.includes('/')
        ? entry.relativePath.substring(0, entry.relativePath.lastIndexOf('/'))
        : ''
      const uf = { name: entry.relativePath, progress: 0, status: '', speed: '' }
      uploadingFiles.value.push(uf)

      try {
        const formData = new FormData()
        formData.append('files', entry.file)

        const startTime = Date.now()
        const resp = await http.post('/api/upload', formData, {
          params: { path: basePath ? basePath + '/' + dir : dir },
          headers: { 'Content-Type': 'multipart/form-data' },
          timeout: 0,
          onUploadProgress: (e) => {
            if (e.total > 0) {
              uf.progress = Math.round((e.loaded / e.total) * 100)
              const elapsed = (Date.now() - startTime) / 1000
              if (elapsed > 0.5) {
                const speed = e.loaded / elapsed
                uf.speed = formatSpeed(speed)
              }
            }
          },
        })
        uf.progress = 100
        uf.status = 'success'
      } catch {
        uf.status = 'exception'
      }
    }
  }

  const workers = Array.from({ length: Math.min(CONCURRENT, fileEntries.length) }, () => processNext())
  await Promise.all(workers)

  ElMessage.success(`文件夹上传完成：${fileEntries.length} 个文件`)
  setTimeout(() => {
    uploadingFiles.value = uploadingFiles.value.filter(f => f.status === 'exception')
    if (uploadingFiles.value.length === 0) uploadingFiles.value = []
    loadFiles(currentPath.value)
  }, 1500)
}

// 拖拽事件
function onDragOver() {
  isDragging.value = true
}
function onDragLeave(e) {
  isDragging.value = false
}
function onDrop(e) {
  isDragging.value = false
  const fileList = e.dataTransfer.files
  if (fileList && fileList.length > 0) {
    uploadFiles(Array.from(fileList))
  }
}

async function uploadFiles(fileList) {
  const skippedFiles = []
  const renamedFiles = []
  const CONCURRENT = 2
  let queue = [...fileList]

  async function processNext() {
    while (queue.length > 0) {
      const file = queue.shift()
      if (!file) break

      const uf = { name: file.name, progress: 0, status: '', speed: '' }
      uploadingFiles.value.push(uf)

      try {
        if (file.size > 2 * 1024 * 1024 * 1024) {
          const resp = await uploadFileChunked(file, currentPath.value || '', uf)
          if (resp?.skipped) {
            skippedFiles.push(file.name)
            uf.status = 'skipped'
            continue
          }
          if (resp?.renamed) {
            renamedFiles.push({ from: file.name, to: resp.name })
          }
          uf.progress = 100
          uf.status = 'success'
        } else {
          const formData = new FormData()
          formData.append('files', file)

          const uploadStartTime = Date.now()
          const resp = await http.post('/api/upload', formData, {
            params: { path: currentPath.value || '' },
            headers: { 'Content-Type': 'multipart/form-data' },
            timeout: 0,
            onUploadProgress: (e) => {
              if (e.total > 0) {
                uf.progress = Math.round((e.loaded / e.total) * 100)
                const elapsed = (Date.now() - uploadStartTime) / 1000
                if (elapsed > 0.5) {
                  uf.speed = formatSpeed(e.loaded / elapsed)
                }
              }
            },
          })
          const result = resp.data.data
          if (Array.isArray(result) && result[0]?.skipped) {
            skippedFiles.push(file.name)
            uf.status = 'skipped'
            continue
          }
          if (Array.isArray(result) && result[0]?.renamed) {
            renamedFiles.push({ from: file.name, to: result[0].name })
          }
          uf.progress = 100
          uf.status = 'success'
        }
      } catch {
        uf.status = 'exception'
      }
    }
  }

  const workers = Array.from({ length: Math.min(CONCURRENT, fileList.length) }, () => processNext())
  await Promise.all(workers)

  if (skippedFiles.length > 0) {
    ElMessage.warning(`已跳过 ${skippedFiles.length} 个重复文件：${skippedFiles.join('、')}`)
  }
  if (renamedFiles.length > 0) {
    const msg = renamedFiles.map(r => `${r.from} → ${r.to}`).join('、')
    ElMessage.info(`已重命名 ${renamedFiles.length} 个文件：${msg}`)
  }

  setTimeout(() => {
    const hasFailed = uploadingFiles.value.some(f => f.status === 'exception')
    if (hasFailed) {
      uploadingFiles.value = uploadingFiles.value.filter(f => f.status === 'exception')
    } else {
      uploadingFiles.value = []
    }
    loadFiles(currentPath.value)
  }, 1500)
}

// 分片上传（大文件，>10MB）
async function uploadFileChunked(file, dirPath, uf) {
  const CHUNK_SIZE = 5 * 1024 * 1024
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE)

  // 1. 初始化分片上传，获取 upload_id
  const initResp = await http.post('/api/upload/chunk', {
    file_name: file.name,
    file_size: file.size,
    total_chunks: totalChunks,
    path: dirPath,
  }, { timeout: 0 })
  const uploadId = initResp.data.data.upload_id

  // 2. 并发上传分片（最多3个并发）
  const CONCURRENCY = 3
  let uploadedCount = 0

  async function uploadChunk(index) {
    const start = index * CHUNK_SIZE
    const end = Math.min(start + CHUNK_SIZE, file.size)
    const chunk = file.slice(start, end)

    const formData = new FormData()
    formData.append('chunk_index', index)
    formData.append('chunk', chunk)

    await http.post(`/api/upload/chunk/${uploadId}`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
    })

    uploadedCount++
    uf.progress = Math.round((uploadedCount / totalChunks) * 100)
  }

  // Promise pool 模式控制并发
  const indices = Array.from({ length: totalChunks }, (_, i) => i)
  for (let i = 0; i < indices.length; i += CONCURRENCY) {
    const batch = indices.slice(i, i + CONCURRENCY)
    await Promise.all(batch.map(uploadChunk))
  }

  // 3. 完成分片上传，合并文件
  const completeResp = await http.post(`/api/upload/chunk/${uploadId}/complete`, null, { timeout: 0 })
  return completeResp.data.data
}

// === 新建文件夹 ===

function showMkdirDialog() {
  mkdirName.value = ''
  mkdirDialogVisible.value = true
}

async function doMkdir() {
  if (!mkdirName.value.trim()) {
    ElMessage.warning('请输入文件夹名称')
    return
  }
  mkdirLoading.value = true
  try {
    await http.post('/api/mkdir', {
      path: currentPath.value || '',
      name: mkdirName.value.trim(),
    })
    ElMessage.success('创建成功')
    mkdirDialogVisible.value = false
    loadFiles(currentPath.value)
  } catch {
    // 错误已由拦截器处理
  } finally {
    mkdirLoading.value = false
  }
}

// === 删除 ===

async function handleDelete(file) {
  try {
    await ElMessageBox.confirm(
      `确定要删除「${file.name}」吗？文件将移入回收站。`,
      '确认删除',
      { type: 'warning' }
    )
    await http.post('/api/delete', { uuid: file.uuid })
    ElMessage.success('已移入回收站')
    loadFiles(currentPath.value)
  } catch {
    // 用户取消
  }
}

// === 右键菜单 ===

function showContextMenu(e, file) {
  contextFile.value = file
  contextMenuX.value = e.clientX
  contextMenuY.value = e.clientY
  contextMenuVisible.value = true
  requestAnimationFrame(() => {
    const el = document.querySelector('.context-menu')
    if (!el) return
    const rect = el.getBoundingClientRect()
    if (rect.right > window.innerWidth) {
      contextMenuX.value = window.innerWidth - rect.width - 4
    }
    if (rect.bottom > window.innerHeight) {
      contextMenuY.value = window.innerHeight - rect.height - 4
    }
  })
}

function hideContextMenu() {
  contextMenuVisible.value = false
  contextFile.value = null
}

function handleContextOpen() {
  const file = contextFile.value
  hideContextMenu()
  if (file) handleClick(file)
}

function handleContextDownload() {
  const file = contextFile.value
  hideContextMenu()
  if (file) triggerDownload(file)
}

function handleContextPotPlayer() {
  const file = contextFile.value
  hideContextMenu()
  if (file) openInPotPlayer(file)
}

function handleContextDelete() {
  const file = contextFile.value
  hideContextMenu()
  if (file) handleDelete(file)
}

function handleContextRegenThumb() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  http.post('/api/file/regen-thumb', { uuid: file.uuid }).then(() => {
    ElMessage.success('已提交缩略图重新生成任务')
  }).catch(err => {
    ElMessage.error('重新生成失败: ' + (err.response?.data?.message || err.message))
  })
}

function handleContextVideoRepair() {
  const file = contextFile.value
  hideContextMenu()
  if (!file) return
  const loadingMsg = ElMessage({ message: '正在修复视频，请耐心等待…', type: 'info', duration: 0 })
  http.post('/api/video/repair', { uuid: file.uuid }, { timeout: 0 }).then((res) => {
    loadingMsg.close()
    const newSize = res.data?.data?.new_size
    ElMessage.success('视频修复完成' + (newSize ? '（新大小: ' + formatSize(newSize) + '）' : ''))
    // 刷新列表以更新文件大小
    fetchFiles()
  }).catch(err => {
    loadingMsg.close()
    ElMessage.error('视频修复失败: ' + (err.response?.data?.message || err.message))
  })
}

// 格式化文件大小
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

// 格式化上传速度
function formatSpeed(bytesPerSec) {
  if (bytesPerSec < 1024) return bytesPerSec.toFixed(0) + ' B/s'
  if (bytesPerSec < 1024 * 1024) return (bytesPerSec / 1024).toFixed(1) + ' KB/s'
  return (bytesPerSec / 1024 / 1024).toFixed(1) + ' MB/s'
}

// === 键盘快捷键 ===

function isInputFocused() {
  const el = document.activeElement
  if (!el) return false
  const tag = el.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || el.isContentEditable
}

function onKeyDown(e) {
  if (isInputFocused()) return

  // Escape
  if (e.key === 'Escape') {
    if (batchMode.value) {
      exitBatchMode()
    } else if (isSearching.value) {
      clearSearch()
    }
    return
  }

  // Delete / Backspace
  if (e.key === 'Delete' || e.key === 'Backspace') {
    if (selectedFiles.value.size > 0 && batchMode.value) {
      e.preventDefault()
      batchDelete()
    }
    return
  }

  // F2 重命名
  if (e.key === 'F2') {
    e.preventDefault()
    if (selectedFiles.value.size === 1 && !batchMode.value) {
      const uuid = Array.from(selectedFiles.value)[0]
      const file = files.value.find(f => f.uuid === uuid)
      if (file) {
        renameFile.value = file
        renameNewName.value = file.name
        renameDialogVisible.value = true
      }
    }
    return
  }

  // Ctrl+A 全选
  if ((e.ctrlKey || e.metaKey) && e.key === 'a') {
    e.preventDefault()
    if (batchMode.value) {
      toggleSelectAll()
    } else {
      enterBatchMode()
      selectedFiles.value = new Set(files.value.map(f => f.uuid))
    }
    return
  }

  // Ctrl+C 复制
  if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
    if (selectedFiles.value.size > 0 || contextFile.value) {
      e.preventDefault()
      if (selectedFiles.value.size > 0) {
        clipboard.type = 'copy'
        clipboard.files = files.value.filter(f => selectedFiles.value.has(f.uuid))
        ElMessage.success(`已复制 ${clipboard.files.length} 个文件`)
      } else if (contextFile.value) {
        clipboard.type = 'copy'
        clipboard.files = [contextFile.value]
        ElMessage.success('已复制到剪贴板')
      }
    }
    return
  }

  // Ctrl+X 剪切
  if ((e.ctrlKey || e.metaKey) && e.key === 'x') {
    if (selectedFiles.value.size > 0 || contextFile.value) {
      e.preventDefault()
      if (selectedFiles.value.size > 0) {
        clipboard.type = 'cut'
        clipboard.files = files.value.filter(f => selectedFiles.value.has(f.uuid))
        ElMessage.success(`已剪切 ${clipboard.files.length} 个文件`)
      } else if (contextFile.value) {
        clipboard.type = 'cut'
        clipboard.files = [contextFile.value]
        ElMessage.success('已剪切到剪贴板')
      }
    }
    return
  }

  // Ctrl+V 粘贴
  if ((e.ctrlKey || e.metaKey) && e.key === 'v') {
    if (clipboard.files.length > 0) {
      e.preventDefault()
      pasteFromClipboard()
    }
    return
  }
}

// 监听路由变化
watch(
  () => route.query.path,
  (newPath, oldPath) => {
    exitBatchMode()
    isSearching.value = false
    savedFiles.value = []
    const page = parseInt(route.query.page) || 1
    currentPage.value = page
    const direction = navDirection
    navDirection = 'reset'
    loadFiles(route.query.path || '', page).then(() => {
      if (oldPath === undefined) return // 首次加载不处理
      nextTick(() => {
        if (direction === 'back') {
          // 返回上级：恢复之前保存的滚动位置
          const saved = scrollPositionStack.pop()
          scrollToPosition(saved ? saved.scrollTop : 0)
        } else {
          // 进入子目录或其他跳转：滚到顶部
          scrollToTop()
        }
      })
    })
  },
  { immediate: true }
)

watch(viewMode, v => localStorage.setItem('viewMode', v))
watch(sortBy, v => localStorage.setItem('sortBy', v))
watch(sortOrder, v => localStorage.setItem('sortOrder', v))
watch(filterType, v => localStorage.setItem('filterType', v))

function onGlobalClick() {
  hideContextMenu()
}

onMounted(() => {
  document.addEventListener('click', onGlobalClick)
  document.addEventListener('keydown', onKeyDown)
  initDarkMode()
  loadStats()
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
  document.removeEventListener('keydown', onKeyDown)
})
</script>

<style scoped>
.file-manager {
  width: 100%;
  max-width: 100vw;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--fm-bg, #f5f7fa);
  overflow-x: hidden;
}

.navbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  background: var(--fm-navbar-bg, #fff);
  border-bottom: 1px solid var(--fm-border-color, #e4e7ed);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
  gap: 8px;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 100;
}

.navbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 1;
  min-width: 0;
  overflow: hidden;
}

.search-input {
  width: 200px;
  flex-shrink: 0;
}

/* 面包屑溢出省略 */
.navbar-left :deep(.el-breadcrumb) {
  min-width: 0;
  overflow: hidden;
  flex-shrink: 1;
}
.navbar-left :deep(.el-breadcrumb__item) {
  max-width: 150px;
}
.navbar-left :deep(.el-breadcrumb__inner) {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
}

.navbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  flex-wrap: wrap;
}

.breadcrumb-link {
  cursor: pointer;
  color: var(--el-color-primary);
}
.breadcrumb-link:hover {
  text-decoration: underline;
}

/* 桌面端显示按钮文字 */
.btn-text {
  display: inline;
}

/* 桌面端/移动端控制 */
.desktop-only {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.mobile-only {
  display: none;
}

.stats-text {
  font-size: 12px;
  color: var(--fm-text-secondary, #909399);
  white-space: nowrap;
  flex-shrink: 0;
}

.file-content {
  flex: 1;
  padding: 20px 24px;
  padding-top: 72px;
  position: relative;
  transition: background 0.2s;
  min-height: 400px;
  max-width: 100%;
  overflow-x: hidden;
}

.file-content.drag-active {
  background: #ecf5ff;
  outline: 3px dashed #409eff;
  outline-offset: -3px;
}

.drag-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 100;
  background: rgba(255, 255, 255, 0.85);
  pointer-events: none;
}
.drag-overlay p {
  margin-top: 16px;
  font-size: 18px;
  color: #409eff;
  font-weight: 500;
}

.upload-toast {
  position: fixed;
  top: 72px;
  right: 16px;
  width: 380px;
  max-height: 320px;
  background: var(--fm-card-bg, #fff);
  border-radius: 10px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.15);
  z-index: 2000;
  overflow: hidden;
  border: 1px solid var(--fm-border-color, #e4e7ed);
}
.upload-toast-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  border-bottom: 1px solid var(--fm-border-color, #e4e7ed);
  font-size: 13px;
  font-weight: 600;
  color: var(--fm-text-primary, #303133);
}
.upload-toast-body {
  max-height: 260px;
  overflow-y: auto;
  padding: 8px 14px;
}
.upload-toast-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
}
.upload-name {
  font-size: 12px;
  color: var(--fm-text-secondary, #606266);
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.upload-skipped-tag {
  font-size: 11px;
  color: #e6a23c;
  white-space: nowrap;
}
.upload-speed {
  font-size: 11px;
  color: #67c23a;
  white-space: nowrap;
}
.upload-slide-enter-active, .upload-slide-leave-active {
  transition: all 0.3s ease;
}
.upload-slide-enter-from, .upload-slide-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

html.dark .upload-toast {
  background: #1d1d1d;
  border-color: #3a3a3a;
}
html.dark .upload-toast-header {
  border-color: #3a3a3a;
  color: #e5eaf3;
}
html.dark .upload-speed {
  color: #67c23a;
}

.search-hint {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  margin-bottom: 16px;
  background: var(--fm-card-bg, #fff);
  border-radius: 8px;
  font-size: 14px;
  color: var(--fm-text-primary, #303133);
}

.search-filters {
  padding: 8px 16px;
  margin-bottom: 16px;
}

.recent-visits {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.recent-label {
  font-size: 13px;
  color: var(--fm-text-secondary, #909399);
  white-space: nowrap;
}

.go-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  margin-bottom: 16px;
  color: var(--fm-text-secondary, #606266);
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.2s;
}
.go-back:hover {
  background: var(--fm-hover-bg, #e8eaed);
  color: var(--el-color-primary);
}

/* 桌面端：一行6个卡片 */
.file-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 16px;
  width: 100%;
}

.file-card {
  background: var(--fm-card-bg, #fff);
  border-radius: 10px;
  padding: 14px;
  cursor: pointer;
  transition: all 0.2s;
  border: 2px solid transparent;
  user-select: none;
  position: relative;
  min-width: 0;
  overflow: hidden;
}
.file-card:hover {
  border-color: var(--el-color-primary);
  box-shadow: 0 2px 12px rgba(64, 158, 255, 0.15);
  transform: translateY(-2px);
}
.file-card-selected {
  border-color: var(--el-color-primary);
  background: var(--fm-selected-bg, #ecf5ff);
}

.file-card-checkbox {
  position: absolute;
  top: 6px;
  left: 6px;
  z-index: 10;
  background: rgba(255, 255, 255, 0.9);
  border-radius: 4px;
  padding: 2px;
}

.file-card-thumb {
  width: 100%;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: var(--fm-thumb-bg, #fafafa);
  border-radius: 8px;
  margin-bottom: 10px;
}
.file-card-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  pointer-events: none;
  -webkit-user-drag: none;
  user-select: none;
}

.file-card-name {
  font-size: 13px;
  color: var(--fm-text-primary, #303133);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: center;
  margin-bottom: 2px;
}

.file-card-sub {
  font-size: 11px;
  color: var(--fm-text-secondary, #909399);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: center;
  margin-bottom: 2px;
}

.file-card-info {
  font-size: 11px;
  color: var(--fm-text-secondary, #909399);
  text-align: center;
}

.skeleton-thumb {
  width: 100%;
  aspect-ratio: 1;
  background: linear-gradient(90deg, #eee 25%, #f5f5f5 50%, #eee 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 8px;
  margin-bottom: 10px;
}
html.dark .skeleton-thumb {
  background: linear-gradient(90deg, #2a2a2a 25%, #333 50%, #2a2a2a 75%);
  background-size: 200% 100%;
}
.skeleton-line {
  height: 14px;
  background: linear-gradient(90deg, #eee 25%, #f5f5f5 50%, #eee 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 4px;
  margin-bottom: 8px;
}
html.dark .skeleton-line {
  background: linear-gradient(90deg, #2a2a2a 25%, #333 50%, #2a2a2a 75%);
  background-size: 200% 100%;
}
.skeleton-line.short {
  width: 60%;
  margin: 0 auto;
}
.skeleton-card {
  cursor: default;
  pointer-events: none;
  border-color: transparent !important;
}
.skeleton-card:hover {
  transform: none !important;
  box-shadow: none !important;
  border-color: transparent !important;
}
@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}

.list-file-name {
  display: flex;
  align-items: center;
  gap: 8px;
}

.list-file-path {
  font-size: 11px;
  color: var(--fm-text-secondary, #909399);
  margin-top: 2px;
}

.video-player {
  width: 100%;
  max-height: 65vh;
  background: #000;
  border-radius: 4px;
  outline: none;
}

.video-speed-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  padding: 6px 0;
}

.video-error-tip {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  padding: 40px 20px;
  background: #fafafa;
  border-radius: 4px;
}

.video-transcode-tip {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  padding: 40px 20px;
  background: #f5f7fa;
  border-radius: 4px;
}

@keyframes transcode-rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.transcode-spin {
  animation: transcode-rotate 1.5s linear infinite;
}

.preview-content {
  min-height: 300px;
  max-height: 70vh;
}

.preview-iframe {
  width: 100%;
  height: 70vh;
  border: none;
}

.preview-text {
  background: var(--fm-thumb-bg, #fafafa);
  border: 1px solid var(--fm-border-color, #e4e7ed);
  border-radius: 6px;
  padding: 16px;
  font-size: 13px;
  line-height: 1.6;
  max-height: 70vh;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
}

html.dark .preview-text {
  color: #e5eaf3;
  background: #262626;
  border-color: #3a3a3a;
}

.pagination-bar {
  display: flex;
  justify-content: center;
  padding: 16px 0 8px;
  overflow-anchor: none;
}

@media (max-width: 768px) {
  .pagination-bar {
    padding: 12px 0 4px;
  }
  .pagination-bar :deep(.el-pagination) {
    flex-wrap: wrap;
    justify-content: center;
  }
}

.batch-bottom-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 24px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.08);
}
html.dark .batch-bottom-bar {
  background: #1d1e1f;
  border-top-color: #414243;
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.3);
}
.batch-info {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-right: 8px;
}
html.dark .batch-info {
  color: #e5eaf3;
}

.audio-player-wrapper {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 20px;
}

.mini-audio-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 999;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 20px;
  background: var(--fm-card-bg, #fff);
  border-top: 1px solid var(--fm-border-color, #e4e7ed);
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.1);
}
.mini-audio-title {
  font-size: 13px;
  color: var(--fm-text-primary, #303133);
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mini-audio-player {
  flex: 1;
  height: 36px;
}
.audio-slide-enter-active, .audio-slide-leave-active {
  transition: all 0.3s ease;
}
.audio-slide-enter-from, .audio-slide-leave-to {
  transform: translateY(100%);
}

html.dark .mini-audio-bar {
  background: #1d1d1d;
  border-color: #3a3a3a;
}

/* 移动/复制对话框 */
.move-dialog-content p {
  margin-bottom: 8px;
  font-size: 14px;
  color: var(--fm-text-primary, #303133);
}

.dir-browser {
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  overflow: hidden;
}
.dir-current {
  padding: 8px 12px;
  background: #f5f7fa;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
}
.dir-current:hover { background: #ecf5ff; }
.dir-breadcrumb {
  padding: 4px 12px;
  font-size: 12px;
  color: #606266;
  background: #fafafa;
  border-top: 1px solid #ebeef5;
}
.dir-seg { cursor: pointer; }
.dir-seg:hover { color: #409eff; }
.dir-list {
  max-height: 240px;
  overflow-y: auto;
  border-top: 1px solid #ebeef5;
}
.dir-item {
  padding: 8px 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 13px;
}
.dir-item:hover { background: #f5f7fa; }
.dir-item.active { background: #ecf5ff; color: #409eff; }
.dir-empty {
  padding: 20px;
  text-align: center;
  color: #909399;
  font-size: 13px;
}

/* 文件详情 */
.file-detail {
  padding: 0 20px;
}
.detail-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid var(--fm-border-color, #e4e7ed);
}
.detail-label {
  font-size: 14px;
  color: var(--fm-text-secondary, #909399);
  flex-shrink: 0;
  margin-right: 12px;
}
.detail-value {
  font-size: 14px;
  color: var(--fm-text-primary, #303133);
  text-align: right;
  word-break: break-all;
}
.detail-footer {
  margin-top: 24px;
  text-align: center;
}

.context-menu {
  position: fixed;
  z-index: 9999;
  background: var(--fm-card-bg, #fff);
  border-radius: 6px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  padding: 4px 0;
  min-width: 140px;
}
.context-menu-item {
  padding: 8px 16px;
  cursor: pointer;
  font-size: 13px;
  color: var(--fm-text-primary, #303133);
  transition: background 0.15s;
}
.context-menu-item:hover {
  background: var(--fm-hover-bg, #f0f2f5);
}
.context-menu-item.danger {
  color: #f56c6c;
}
.context-menu-item.danger:hover {
  background: #fef0f0;
}
.context-menu-divider {
  height: 1px;
  background: var(--fm-border-color, #e4e7ed);
  margin: 4px 0;
}

/* ===== 移动端适配 ===== */
@media (max-width: 768px) {
  .navbar {
    padding: 10px 12px;
    gap: 6px;
  }
  .navbar-left {
    gap: 6px;
  }
  .navbar-left :deep(.el-breadcrumb__item) {
    max-width: 80px;
  }
  .navbar-right {
    gap: 4px;
  }
  /* 手机端隐藏按钮文字，只显示图标 */
  .btn-text {
    display: none;
  }
  .desktop-only {
    display: none !important;
  }
  .mobile-only {
    display: inline-block !important;
  }
  .navbar-right :deep(.el-button) {
    padding: 8px;
    font-size: 16px;
  }
  .search-input {
    width: 140px;
  }
  .stats-text {
    display: none;
  }
  .upload-toast {
    width: calc(100vw - 20px);
    right: 10px;
    top: 64px;
  }

  .file-content {
    padding: 10px;
  }

  /* 手机：一行2个卡片 */
  .file-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }

  .file-card {
    padding: 8px;
    border-radius: 8px;
  }

  .file-card-name {
    font-size: 12px;
  }
  .file-card-info {
    font-size: 10px;
  }

  /* el-table 横向滚动 */
  .file-content :deep(.el-table) {
    width: 100% !important;
    display: block;
    overflow-x: auto;
  }

  /* 视频播放器全宽 */
  .video-player {
    max-height: 50vh;
  }
}

/* 平板适配 */
@media (min-width: 769px) and (max-width: 1200px) {
  .file-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}
</style>

<style>
.el-table .row-selected {
  background-color: #ecf5ff !important;
}
.el-table .row-selected:hover > td {
  background-color: #d9ecff !important;
}

/* 暗色模式变量覆盖 */
html.dark {
  --fm-bg: #141414;
  --fm-navbar-bg: #1d1d1d;
  --fm-border-color: #3a3a3a;
  --fm-card-bg: #1d1d1d;
  --fm-text-primary: #e5eaf3;
  --fm-text-secondary: #a3a6ad;
  --fm-selected-bg: #18222c;
  --fm-hover-bg: #2a2a2a;
  --fm-thumb-bg: #262626;
}

html.dark .el-dialog {
  --el-dialog-bg-color: #1d1d1d;
}

html.dark .el-drawer {
  --el-drawer-bg-color: #1d1d1d;
}

html.dark .el-overlay {
  --el-overlay-color-lighter: rgba(0, 0, 0, 0.7);
}

html.dark .el-input__wrapper {
  background-color: #2a2a2a;
  box-shadow: 0 0 0 1px #3a3a3a inset;
}

html.dark .el-input__inner {
  color: #e5eaf3;
}

html.dark .el-button {
  --el-button-text-color: #e5eaf3;
}

html.dark .el-dropdown-menu {
  background-color: #1d1d1d;
  border-color: #3a3a3a;
}

html.dark .el-dropdown-menu__item {
  color: #e5eaf3;
}

html.dark .el-dropdown-menu__item:hover {
  background-color: #2a2a2a;
}

html.dark .el-dropdown-menu__item.is-active {
  color: var(--el-color-primary);
}

html.dark .el-empty__description p {
  color: #a3a6ad;
}

html.dark .el-breadcrumb__inner {
  color: #a3a6ad;
}

html.dark .el-breadcrumb__separator {
  color: #5a5a5a;
}

html.dark .el-checkbox__label {
  color: #e5eaf3;
}

html.dark .el-progress__text {
  color: #e5eaf3;
}

html.dark .context-menu {
  background: #1d1d1d;
  border: 1px solid #3a3a3a;
}

html.dark .context-menu-item {
  color: #e5eaf3;
}

html.dark .context-menu-item:hover {
  background: #2a2a2a;
}

html.dark .context-menu-divider {
  background: #3a3a3a;
}

html.dark .drag-overlay {
  background: rgba(20, 20, 20, 0.85);
}
</style>
