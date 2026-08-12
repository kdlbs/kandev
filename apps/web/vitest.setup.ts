import type { Window as HappyDOMWindow } from "happy-dom";
import * as React from "react";

import { initI18nForTests, loadAllLocalesForTests } from "./lib/i18n";

/**
 * Fail with the diagnosis instead of the symptom.
 *
 * `act` is exported only by React's development build. Without it,
 * `@testing-library/react`'s act-compat shim quietly falls back to the
 * deprecated `react-dom/test-utils`, whose production build calls `React.act`
 * and throws `TypeError: React.act is not a function` from inside
 * `node_modules` on the first `render()` — in every suite at once, naming
 * nothing that would lead you to the cause. That read as an environment quirk
 * for as long as it went unfixed.
 *
 * `vitest.config.ts` pins `NODE_ENV=test`, which is the known cause. This guard
 * stays because it also covers the ones that pin cannot: a duplicated `react`
 * instance, or an alias resolving the shim and the components under test to
 * different copies.
 *
 * A namespace import is deliberate — a named `import { act }` fails differently
 * against the CJS production build, which is the case being diagnosed.
 */
if (typeof React.act !== "function") {
  throw new Error(
    `React.act is unavailable, so no test can render. React only exports act() from its ` +
      `development build, and NODE_ENV is "${process.env.NODE_ENV}" here. ` +
      `Check the NODE_ENV pin in vitest.config.ts, then that only one copy of react is installed ` +
      `(pnpm why react). Without it @testing-library/react falls back to react-dom/test-utils and ` +
      `every render() throws "TypeError: React.act is not a function".`,
  );
}

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
