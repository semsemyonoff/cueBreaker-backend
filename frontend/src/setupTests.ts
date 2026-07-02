import '@testing-library/jest-dom/vitest'

// Node 22+'s built-in global `localStorage` shadows jsdom's window.localStorage
// under Vitest's jsdom environment, leaving `window.localStorage` undefined.
// Replace it with a minimal in-memory polyfill so code under test can rely on
// `window.localStorage` the same way it would in a real browser.
if (!window.localStorage) {
  const store = new Map<string, string>()
  const polyfill: Storage = {
    getItem: (key) => store.get(key) ?? null,
    setItem: (key, value) => {
      store.set(key, String(value))
    },
    removeItem: (key) => {
      store.delete(key)
    },
    clear: () => {
      store.clear()
    },
    key: (index) => Array.from(store.keys())[index] ?? null,
    get length() {
      return store.size
    },
  }
  Object.defineProperty(window, 'localStorage', { value: polyfill, configurable: true })
  Object.defineProperty(globalThis, 'localStorage', { value: polyfill, configurable: true })
}
