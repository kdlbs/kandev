import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

/**
 * Where `/settings` lands when this device has never opened a settings page.
 * The first entry of the General group, which is also what the General group
 * header points at — see `components/settings/general-nav.ts`.
 */
export const DEFAULT_SETTINGS_PATH = "/settings/general/appearance";

/**
 * A path segment that identifies a record (workspace, agent, executor or plugin
 * id) rather than a page. Same shape the settings breadcrumb already skips when
 * deriving a page title.
 */
const ID_SEGMENT = /^[0-9a-f-]{8,}$/i;

function normalize(pathname: string): string {
  return pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
}

/**
 * True when `pathname` is worth restoring the next time someone opens bare
 * `/settings`.
 *
 * Two exclusions, and only two:
 *
 * - **`/settings` itself**, or it would restore to itself.
 * - **Anything carrying a record id** (`/settings/workspace/<id>/repositories`,
 *   `/settings/agents/<id>`). Deciding whether one of those still resolves needs
 *   the workspace/agent list, not a route table, and restoring a page for a
 *   deleted record is worse than restoring the default.
 *
 * Redirect stubs (`/settings/general/shell`, `/settings/system`, …) are
 * deliberately *not* excluded: they replace the URL on mount, so the very next
 * thing recorded is the page they land on.
 */
export function isRememberableSettingsPath(pathname: string): boolean {
  const path = normalize(pathname);
  if (!path.startsWith("/settings/")) return false;
  return !path.split("/").some((segment) => ID_SEGMENT.test(segment));
}

/** Record the settings page to return to. No-ops for paths we won't restore. */
export function rememberSettingsPath(pathname: string): void {
  const path = normalize(pathname);
  if (!isRememberableSettingsPath(path)) return;
  setLocalStorage(STORAGE_KEYS.LAST_SETTINGS_PATH, path);
}

/**
 * The settings page bare `/settings` should resolve to on this device.
 *
 * Re-validated on read rather than trusted: the stored value may predate a
 * release that moved or removed that page, or have been written by hand.
 */
export function readLastSettingsPath(): string {
  const stored = getLocalStorage(STORAGE_KEYS.LAST_SETTINGS_PATH, "");
  return isRememberableSettingsPath(stored) ? normalize(stored) : DEFAULT_SETTINGS_PATH;
}
