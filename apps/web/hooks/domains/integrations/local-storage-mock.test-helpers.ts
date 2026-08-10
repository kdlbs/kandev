/**
 * Shared in-memory localStorage mock for the per-integration "enabled" hook
 * test suites (azure-devops, github, gitlab, hide-disabled-in-nav). Each
 * test file calls this factory to build its own mock instance — the store
 * is never shared across files, so suites stay isolated from one another
 * even though they all use the same shape.
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
