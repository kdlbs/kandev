import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

/**
 * Where `/settings` lands when this device has never opened a settings page.
 * The first entry of the General group, which is also what the General group
 * header points at — see `components/settings/general-nav.ts`.
 */
export const DEFAULT_SETTINGS_PATH = "/settings/general/appearance";

const SETTINGS_INDEX_PATH = "/settings";

function normalize(pathname: string): string {
  return pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
}

/**
 * True when `pathname` is worth restoring the next time someone opens bare
 * `/settings`.
 *
 * `knownPaths` is the static settings route table (`SETTINGS_ROUTE_PATHS`), and
 * membership in it is the whole test. Restoring is a promise that the page will
 * still be there, and only a static first-party route can keep it:
 *
 * - **Unknown paths** (`/settings/does-not-exist`, a route dropped in an
 *   upgrade) would otherwise stick, because the settings shell renders — and so
 *   records — even when the route falls through to `SettingsRouteFallback`.
 * - **Record and slug routes** (`/settings/workspace/<id>/repositories`,
 *   `/settings/agents/<name>`, a plugin's `/settings/plugins/<slug>`) resolve
 *   against data that can disappear. Deciding whether one still resolves needs
 *   the workspace/agent/plugin list, and landing on a page for something that
 *   was deleted is worse than landing on the default.
 * - **`/settings` itself** is a route, so it needs excluding by hand: it is the
 *   route doing the restoring.
 *
 * Redirect stubs (`/settings/general/shell`, `/settings/system`, …) are
 * deliberately kept: they are static routes that replace the URL on mount, so
 * the next thing recorded is the page they land on.
 */
export function isRestorableSettingsPath(
  pathname: string,
  knownPaths: ReadonlySet<string>,
): boolean {
  const path = normalize(pathname);
  if (path === SETTINGS_INDEX_PATH) return false;
  return knownPaths.has(path);
}

/** Record the settings page to return to. No-ops for paths we won't restore. */
export function rememberSettingsPath(pathname: string, knownPaths: ReadonlySet<string>): void {
  const path = normalize(pathname);
  if (!isRestorableSettingsPath(path, knownPaths)) return;
  setLocalStorage(STORAGE_KEYS.LAST_SETTINGS_PATH, path);
}

/**
 * The settings page bare `/settings` should resolve to on this device.
 *
 * Re-validated against `knownPaths` rather than trusted, so a page that existed
 * when the value was written but has since moved or been removed falls back
 * instead of resolving to a not-found surface forever.
 */
export function readLastSettingsPath(knownPaths: ReadonlySet<string>): string {
  const stored = getLocalStorage(STORAGE_KEYS.LAST_SETTINGS_PATH, "");
  return isRestorableSettingsPath(stored, knownPaths) ? normalize(stored) : DEFAULT_SETTINGS_PATH;
}
