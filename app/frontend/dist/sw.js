const CACHE = 'xingfeng-v1'
self.addEventListener('install', e => { self.skipWaiting() })
self.addEventListener('activate', e => { e.waitUntil(self.clients.claim()) })
self.addEventListener('fetch', e => {
  e.respondWith(
    caches.match(e.request).then(r => r || fetch(e.request).then(resp => {
      if (resp.ok) {
        const clone = resp.clone()
        caches.open(CACHE).then(c => c.put(e.request, clone))
      }
      return resp
    }))
  )
})
