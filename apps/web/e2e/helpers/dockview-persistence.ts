// Waiting on the dockview layout having been *saved*.
//
// Dockview persists the layout on a ~300ms debounce, to `sessionStorage` under
// `kandev.dockview.env-layout-v3.<envId>`. The debounce publishes no event, so a
// spec that reloads to prove the layout survived has nothing to wait for and
// historically slept past the debounce instead — which fails the wrong way
// round: on a loaded shard the save is late, the reload takes the old JSON, and
// the test fails claiming the layout was not preserved.
//
// The debounce leaves an artifact even though it fires no event, so wait on the
// artifact. `snapshotPersistedLayouts` before the action that changes the
// layout, `waitForPersistedLayoutChange` after it, and the reload then reads a
// save that has demonstrably happened.
//
// Every key is read rather than one session's, so callers do not need an
// environment id: a spec drives one task at a time, and any write is the write
// being waited for.
import { expect, type Page } from "@playwright/test";

/**
 * Copy of `DOCKVIEW_ENV_LAYOUT_PREFIX` in `apps/web/lib/local-storage.ts`, which
 * is the authoritative definition — bump both together.
 *
 * Importing it instead would be better, and does not work: `local-storage.ts`
 * pulls in `lib/api/domains/attachment-api.ts` -> `lib/config.ts`, which uses
 * `import.meta`, and Playwright's CJS transform rejects that at load time
 * ("Cannot use 'import.meta' outside a module") before any test is collected.
 * Only type-only imports reach app source from here for that reason.
 *
 * A silent drift is what the copy risks, so it does not stay silent:
 * {@link waitForPersistedLayoutChange} reports the live `sessionStorage` keys on
 * timeout, which distinguishes a stale prefix here from the layout genuinely
 * never being persisted.
 */
const LAYOUT_KEY_PREFIX = "kandev.dockview.env-layout-v3.";

/** Serialise every persisted dockview layout currently in `sessionStorage`. */
export async function snapshotPersistedLayouts(page: Page): Promise<string> {
  return page.evaluate((prefix) => {
    const entries: string[] = [];
    for (let i = 0; i < window.sessionStorage.length; i++) {
      const key = window.sessionStorage.key(i);
      if (!key?.startsWith(prefix)) continue;
      entries.push(`${key}=${window.sessionStorage.getItem(key) ?? ""}`);
    }
    return entries.sort().join("\n");
  }, LAYOUT_KEY_PREFIX);
}

/**
 * Wait until the persisted layout differs from `previous`, i.e. the debounced
 * save has actually run.
 *
 * Fails if no save lands, which is the point: a layout change that never
 * reaches storage is exactly the bug the reload in these specs is checking for,
 * and a sleep would have carried on into a misleading assertion instead.
 *
 * The two ways that failure can happen are indistinguishable from the snapshot
 * alone — the app stopped persisting, or {@link LAYOUT_KEY_PREFIX} drifted from
 * the production constant and this polled a key nothing writes — so the timeout
 * names the keys that were actually present instead of leaving the reader to
 * assume the first.
 */
export async function waitForPersistedLayoutChange(
  page: Page,
  previous: string,
  timeout = 10_000,
): Promise<void> {
  try {
    await expect
      .poll(() => snapshotPersistedLayouts(page), {
        timeout,
        message: "dockview never persisted the layout change",
      })
      .not.toBe(previous);
  } catch (error) {
    const keys = await page.evaluate(() => {
      const found: string[] = [];
      for (let i = 0; i < window.sessionStorage.length; i++) {
        const key = window.sessionStorage.key(i);
        if (key) found.push(key);
      }
      return found.sort();
    });
    const matched = keys.filter((key) => key.startsWith(LAYOUT_KEY_PREFIX));
    const diagnosis =
      matched.length > 0
        ? `${matched.length} key(s) matched "${LAYOUT_KEY_PREFIX}", so the prefix is live and the layout genuinely did not change.`
        : `no key matched "${LAYOUT_KEY_PREFIX}" — this helper's copy of the prefix has probably drifted from DOCKVIEW_ENV_LAYOUT_PREFIX in apps/web/lib/local-storage.ts.`;
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}\n\n${diagnosis}\nsessionStorage keys present: ${keys.join(", ") || "(none)"}`,
    );
  }
}
