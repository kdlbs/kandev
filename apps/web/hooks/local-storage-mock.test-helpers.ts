/**
 * Shared in-memory localStorage mock for the hook test suites that read
 * `localStorage`-backed preferences: the per-integration "enabled" hooks
 * (azure-devops, github, gitlab, jira, linear, sentry), the two nav-visibility
 * toggles (hide-disabled integrations / agent profiles), and the shared
 * `useLocalStorageBoolean` primitive. Lives at the top level of the hooks tree
 * because it is not owned by any one domain. Each test file calls this factory
 * to build its own mock instance — the store is never shared across files, so
 * suites stay isolated from one another even though they all use the same
 * shape.
 */
export function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => {
      store.set(key, value);
    },
    removeItem: (key: string) => {
      store.delete(key);
    },
    clear: () => {
      store.clear();
    },
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}
