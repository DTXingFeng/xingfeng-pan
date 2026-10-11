const CACHE = 'xingfeng-v2'

self.addEventListener('install', event => {
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(keys
        .filter(key => key.startsWith('xingfeng-v') && key !== CACHE)
        .map(key => caches.delete(key))))
      .then(() => self.clients.claim())
  )
})

self.addEventListener('fetch', event => {
  const request = event.request
  const url = new URL(request.url)
  // 上传直接走浏览器网络层，保留 XHR 进度事件；动态数据不能缓存。
  if (request.method !== 'GET' || url.origin !== self.location.origin ||
      /^\/(api|dav)(\/|$)/.test(url.pathname)) return

  function fetchAndCache() {
    return fetch(request).then(response => {
      if (response.ok) {
        const clone = response.clone()
        event.waitUntil(caches.open(CACHE).then(cache => cache.put(request, clone)).catch(() => {}))
      }
      return response
    })
  }

  if (request.mode === 'navigate') {
    // 页面优先取新版本，离线时才使用缓存，避免修复后仍加载旧前端。
    event.respondWith(fetchAndCache().catch(async error => {
      const cached = await caches.match(request, { cacheName: CACHE })
      if (cached) return cached
      throw error
    }))
  } else if (url.pathname.startsWith('/assets/') || url.pathname === '/manifest.json') {
    event.respondWith(caches.match(request, { cacheName: CACHE }).then(cached => cached || fetchAndCache()))
  }
})
