import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'

const source = readFileSync(new URL('../public/sw.js', import.meta.url), 'utf8')

function worker({ offline = false, cacheMiss = false } = {}) {
  const listeners = {}
  const cached = { cached: true }
  const network = { ok: true, clone() { return this } }
  const deleted = []
  const writes = []
  runInNewContext(source, {
    URL,
    self: {
      location: { origin: 'https://pan.example' },
      addEventListener: (event, handler) => { listeners[event] = handler },
      skipWaiting: async () => {},
      clients: { claim: async () => {} },
    },
    caches: {
      match: async () => cacheMiss ? undefined : cached,
      open: async () => ({ put: async request => writes.push(request) }),
      keys: async () => ['xingfeng-v1', 'xingfeng-v2', 'other-app-cache'],
      delete: async name => deleted.push(name),
    },
    fetch: async () => {
      if (offline) throw new Error('Network unavailable')
      return network
    },
  })
  function dispatch(request) {
    let response
    const waits = []
    listeners.fetch({ request, respondWith: value => { response = value }, waitUntil: value => waits.push(value) })
    return { response, waits }
  }
  return { listeners, dispatch, cached, network, deleted, writes }
}

for (const [method, path] of [['POST', '/api/upload'], ['POST', '/api/upload/chunk/123'], ['GET', '/api/list'], ['GET', '/dav/file.txt']]) {
  test(`service worker: ${method} ${path} goes directly to the network`, () => {
    const h = worker()
    const event = h.dispatch({ method, url: `https://pan.example${path}` })
    assert.equal(event.response, undefined, 'API/WebDAV/upload requests must not be intercepted')
  })
}

test('service worker: static assets still use cache', async () => {
  const h = worker()
  const event = h.dispatch({ method: 'GET', url: 'https://pan.example/assets/app.js' })
  assert.equal(await event.response, h.cached)
})

test('service worker: navigation loads fresh HTML instead of the cached app', async () => {
  const h = worker()
  const event = h.dispatch({ method: 'GET', mode: 'navigate', url: 'https://pan.example/files' })
  assert.equal(await event.response, h.network)
  await Promise.all(event.waits)
  assert.equal(h.writes.length, 1)
})

test('service worker: activation removes old app caches and preserves unrelated caches', async () => {
  const h = worker()
  let activation
  h.listeners.activate({ waitUntil: promise => { activation = promise } })
  await activation
  assert.deepEqual(h.deleted, ['xingfeng-v1'])
})

test('service worker: offline navigation can still use cached HTML', async () => {
  const h = worker({ offline: true })
  const event = h.dispatch({ method: 'GET', mode: 'navigate', url: 'https://pan.example/files' })
  assert.equal(await event.response, h.cached)
})

test('service worker: uncached static assets are fetched and stored', async () => {
  const h = worker({ cacheMiss: true })
  const event = h.dispatch({ method: 'GET', url: 'https://pan.example/assets/app.js' })
  assert.equal(await event.response, h.network)
  await Promise.all(event.waits)
  assert.equal(h.writes.length, 1)
})
