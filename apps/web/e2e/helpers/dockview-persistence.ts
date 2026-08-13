import { expect, type Page } from "@playwright/test";

/**
 * Waiting on the dockview layout having been *saved*.
 *
 * Dockview persists the layout on a ~300ms debounce, to `sessionStorage` under
 * `kandev.dockview.env-layout-v3.<envId>`. The debounce publishes no event, so a
 * spec that reloads to prove the layout survived has nothing to wait for and
 * historically slept past the debounce instead — which fails the wrong way
 * round: on a loaded shard the save is late, the reload takes the old JSON, and
 * the test fails claiming the layout was not preserved.
 *
 * The debounce leaves an artifact even though it fires no event, so wait on the
 * artifact. {@link snapshotPersistedLayouts} before the action that changes the
 * layout, {@link waitForPersistedLayoutChange} after it, and the reload then
 * reads a save that has demonstrably happened.
 *
 * Every key is read rather than one session's, so callers do not need an
 * environment id: a spec drives one task at a time, and any write is the write
 * being waited for.
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
 */
export async function waitForPersistedLayoutChange(
  page: Page,
  previous: string,
  timeout = 10_000,
): Promise<void> {
  await expect
    .poll(() => snapshotPersistedLayouts(page), {
      timeout,
      message: "dockview never persisted the layout change",
    })
    .not.toBe(previous);
}
