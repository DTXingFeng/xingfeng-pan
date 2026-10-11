import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { effect, ref, reactive, stop } from 'vue'

// Run the component's real upload handlers with a controlled HTTP transport.
// The render effect catches writes that bypass Vue's reactive proxy.
const source = readFileSync(new URL('../src/views/FileList.vue', import.meta.url), 'utf8')
const handlers = source.split('// === 上传功能 ===')[1].split('// === 新建文件夹 ===')[0]
const speedFormatter = source.match(/function formatSpeed\(bytesPerSec\) \{[\s\S]*?\n\}/)[0]

function harness() {
  const requests = []
  const timers = []
  const messages = []
  const uploadingFiles = ref([])
  const currentPath = ref('')
  let rendered = []
  const render = effect(() => {
    rendered = uploadingFiles.value.map(file => ({ ...file }))
  })
  const http = {
    post(url, body, config) {
      if (url === '/api/mkdir') return Promise.resolve({ data: { data: {} } })
      if (url === '/api/upload/chunk') {
        return Promise.resolve({ data: { data: { upload_id: 'test-upload' } } })
      }
      return new Promise((resolve, reject) => requests.push({ url, body, config, resolve, reject }))
    },
  }
  const api = new Function(
    'http', 'uploadingFiles', 'currentPath', 'ref', 'reactive', 'ElMessage', 'loadFiles', 'setTimeout',
    `${speedFormatter}\n${handlers}\nreturn { uploadFiles, uploadFolderFiles, uploadFileChunked, clearCompletedUploads }`,
  )(
    http, uploadingFiles, currentPath, ref, reactive,
    Object.fromEntries(['success', 'warning', 'info', 'error'].map(level => [level, message => messages.push({ level, message })])),
    async () => {}, callback => timers.push(callback),
  )
  return {
    ...api, requests, uploadingFiles, currentPath, messages,
    rendered: () => rendered,
    flushTimers: () => timers.splice(0).forEach(callback => callback()),
    dispose: () => stop(render),
  }
}

async function until(predicate) {
  for (let i = 0; i < 100; i++) {
    if (predicate()) return
    await Promise.resolve()
  }
  assert.fail('Expected upload request was not started')
}

function file(name = 'example.txt') {
  return new File(['example contents'], name)
}

for (const kind of ['regular', 'folder']) {
  function start(h) {
    const input = file()
    return kind === 'regular'
      ? h.uploadFiles([input])
      : h.uploadFolderFiles(['folder'], [{ file: input, relativePath: `folder/${input.name}` }])
  }

  test(`${kind}: intermediate progress and final status reach the render effect`, async () => {
    const h = harness()
    try {
      const uploading = start(h)
      await until(() => h.requests.length === 1)
      h.requests[0].config.onUploadProgress({ loaded: 25, total: 100 })
      assert.equal(h.rendered()[0].progress, 25, 'UI must react to intermediate upload progress')
      h.requests[0].resolve({ data: { data: [{ name: 'example.txt' }] } })
      await uploading
      assert.equal(h.rendered()[0].status, 'success')
    } finally {
      h.dispose()
    }
  })

  test(`${kind}: completed uploads remain visible after scheduled callbacks`, async () => {
    const h = harness()
    try {
      const uploading = start(h)
      await until(() => h.requests.length === 1)
      h.requests[0].resolve({ data: { data: [{ name: 'example.txt' }] } })
      await uploading
      h.flushTimers()
      assert.equal(h.uploadingFiles.value.length, 1, 'Completed upload must remain until manually cleared')
      assert.equal(h.rendered()[0].status, 'success')
    } finally {
      h.dispose()
    }
  })
}

test('chunked: progress includes bytes sent within concurrent chunks', async () => {
  const h = harness()
  try {
    const task = reactive({ progress: 0, status: '', speed: '' })
    const chunkSize = 5 * 1024 * 1024
    const input = { name: 'big.bin', size: chunkSize * 2 + 1024, slice: (start, end) => new Blob([new Uint8Array(end - start)]) }
    const uploading = h.uploadFileChunked(input, '', task)
    await until(() => h.requests.length === 3)
    assert.equal(typeof h.requests[0].config.onUploadProgress, 'function', 'Each chunk needs byte-level progress')
    h.requests[0].config.onUploadProgress({ loaded: 50, total: 100 })
    assert.ok(task.progress > 0 && task.progress < 100)
    const firstProgress = task.progress
    assert.equal(firstProgress, Math.round(chunkSize / 2 / input.size * 100))
    h.requests[1].config.onUploadProgress({ loaded: 50, total: 100 })
    assert.ok(task.progress > firstProgress, 'Concurrent chunks must contribute to total progress')
    assert.equal(task.progress, Math.round(chunkSize / input.size * 100))
    for (const request of h.requests.slice(0, 3)) request.resolve({ data: { data: {} } })
    await until(() => h.requests.length === 4)
    assert.ok(task.progress < 100, '100% must wait for server confirmation')
    assert.equal(task.phase, 'merging')
    h.requests[3].resolve({ data: { data: { name: input.name } } })
    await uploading
  } finally {
    h.dispose()
  }
})

test('regular: 100% waits for server confirmation and progress never moves backwards', async () => {
  const h = harness()
  try {
    const uploading = h.uploadFiles([file()])
    const request = h.requests[0]
    request.config.onUploadProgress({ loaded: 60, total: 100 })
    request.config.onUploadProgress({ loaded: 20, total: 100 })
    assert.equal(h.rendered()[0].progress, 60)
    request.config.onUploadProgress({ loaded: 100, total: 100 })
    assert.equal(h.rendered()[0].progress, 99)
    assert.equal(h.rendered()[0].phase, 'processing')
    assert.equal(h.rendered()[0].status, '')
    request.resolve({ data: { data: [{}] } })
    await uploading
    assert.equal(h.rendered()[0].progress, 100)
    assert.equal(h.rendered()[0].speed, '')
  } finally {
    h.dispose()
  }
})

test('queue: manual cleanup keeps active/queued tasks and captures the chosen directory', async () => {
  const h = harness()
  try {
    h.currentPath.value = 'target'
    const uploading = h.uploadFiles([file(), file(), file()])
    assert.equal(h.requests.length, 2)
    assert.equal(h.rendered().length, 3)
    assert.equal(h.rendered()[2].phase, 'queued')
    assert.equal(new Set(h.rendered().map(task => task.id)).size, 3, 'Same-name tasks need unique keys')
    h.currentPath.value = 'elsewhere'
    h.requests[0].resolve({ data: { data: [{}] } })
    await until(() => h.requests.length === 3)
    h.clearCompletedUploads()
    assert.equal(h.uploadingFiles.value.length, 2)
    assert.ok(h.rendered().every(task => !task.status))
    assert.ok(h.requests.every(request => request.config.params.path === 'target'))
    h.requests[1].resolve({ data: { data: [{}] } })
    h.requests[2].resolve({ data: { data: [{}] } })
    await uploading
    assert.equal(h.rendered().length, 2)
    h.clearCompletedUploads()
    assert.equal(h.rendered().length, 0)
  } finally {
    h.dispose()
  }
})

test('folder: skipped, failed and renamed results remain visible with an accurate summary', async () => {
  const h = harness()
  try {
    const inputs = ['duplicate.txt', 'failed.txt', 'renamed.txt'].map(name => ({ file: file(name), relativePath: `folder/${name}` }))
    const uploading = h.uploadFolderFiles(['folder'], inputs)
    await until(() => h.requests.length === 2)
    h.requests[0].resolve({ data: { data: [{ skipped: true }] } })
    h.requests[1].reject(new Error('Disk full'))
    await until(() => h.requests.length === 3)
    h.requests[2].resolve({ data: { data: [{ renamed: true, name: 'renamed_1.txt' }] } })
    await uploading
    h.flushTimers()
    assert.deepEqual(h.rendered().map(task => task.status), ['skipped', 'exception', 'success'])
    assert.equal(h.rendered()[0].progress, 100)
    assert.equal(h.rendered()[1].error, 'Disk full')
    assert.equal(h.rendered()[2].renamedName, 'renamed_1.txt')
    assert.ok(h.messages.some(message => message.level === 'warning' && message.message.includes('1 个成功，1 个重复跳过，1 个失败')))
    h.requests[1].config.onUploadProgress({ loaded: 100, total: 100 })
    assert.equal(h.rendered()[1].progress, 0, 'Late callbacks cannot change a failed task')
  } finally {
    h.dispose()
  }
})

test('overlapping batches: finishing a batch preserves other active tasks', async () => {
  const h = harness()
  try {
    const first = h.uploadFiles([file('first.txt')])
    const second = h.uploadFiles([file('second.txt')])
    h.requests[0].resolve({ data: { data: [{}] } })
    await first
    h.flushTimers()
    assert.equal(h.rendered().length, 2)
    assert.equal(h.rendered()[1].status, '')
    h.requests[1].config.onUploadProgress({ loaded: 40, total: 100 })
    assert.equal(h.rendered()[1].progress, 40)
    h.requests[1].resolve({ data: { data: [{}] } })
    await second
  } finally {
    h.dispose()
  }
})
