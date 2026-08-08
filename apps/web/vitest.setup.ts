import type { Window as HappyDOMWindow } from "happy-dom";

import { initI18nForTests, loadAllLocalesForTests } from "./lib/i18n";

/**
 * i18n test bootstrap.
 *
 * Unit tests never render `src/app-shell.tsx`, so nothing initializes i18next
 * for them. react-i18next's `useTranslation` falls back to the default instance
 * when there is no provider in the tree, so initializing here is enough — no
 * render wrapper is needed, and every test's render tree stays byte-identical
 * (including the suites that render via `react-dom/server`).
 *
 * The real `en` catalog is loaded, so assertions on English copy pass exactly as
 * they did before the migration.
 *
 * Only `en` is bundled in the browser; the rest are lazy chunks. Suites drive
 * the shared instance with a bare `i18n.changeLanguage("pseudo")` and assert on
 * the next line, so the non-English catalogs are awaited here — top-level await
 * in a setup file runs before any suite. That keeps unit tests in the
 * "everything in memory" world they were written for without putting the other
 * locales back into the bundle.
 */
initI18nForTests();
await loadAllLocalesForTests();

function createLocalStorageMock(): Storage {
  const store = new Map<string, string>();

  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.has(key) ? (store.get(key) ?? null) : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, value);
    },
  };
}

const localStorageMock = createLocalStorageMock();

Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: localStorageMock,
});

if (typeof window !== "undefined") {
  const happyDOMWindow = window as unknown as HappyDOMWindow;
  happyDOMWindow.happyDOM.settings.fetch.interceptor = {
    beforeAsyncRequest: ({ window: requestWindow }) =>
      Promise.resolve(new requestWindow.Response(null, { status: 404 })),
  };

  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: localStorageMock,
  });
}
