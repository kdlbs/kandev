/**
 * Builds the `ActivePlugin` shape (docs/plans/plugins/PLUGIN-API.md)
 * for a plugin the operator just enabled, so the settings page can trigger a
 * runtime `loadPlugins` call without a full page reload. Mirrors the
 * backend's boot-payload mapping in
 * apps/backend/internal/backendapp/helpers.go (bundleActivePlugins /
 * pluginStyleURLs) — keep both in sync.
 */
import type { ActivePlugin } from "./types";
import type { PluginRecord } from "@/lib/types/plugins";

/**
 * Returns the `ActivePlugin` for a plugin record, or null when the plugin
 * declares no UI bundle (nothing to load).
 *
 * `bundleUrl` carries the installed `version` as a `?v=` query param so an
 * updated plugin resolves to a *different* module specifier. Without this,
 * `unloadPlugin(id, { evictCache: true })` (see `lib/plugins/host.ts`) drops
 * the cached bundle registration, but a same-tab re-`import()` of the same
 * URL returns the browser's already-evaluated ES module without re-running
 * its top-level `registerKandevPlugin()` — leaving the plugin active but
 * unregistered. Keying the URL on version keeps a plain unchanged
 * enable/disable cycle or reload cache-friendly (identical URL, browser
 * cache hit) while forcing a real re-import + re-execution whenever the
 * version actually changes.
 */
export function toActivePlugin(record: PluginRecord): ActivePlugin | null {
  const bundle = record.ui?.bundle;
  if (!bundle) return null;

  return {
    id: record.id,
    name: record.display_name,
    bundleUrl: `/api/plugins/${record.id}/bundle?v=${encodeURIComponent(record.version)}`,
    styleUrls: pluginStyleURLs(record),
  };
}

function pluginStyleURLs(record: PluginRecord): string[] | undefined {
  const styles = record.ui?.styles;
  if (!styles || styles.length === 0) return undefined;
  return styles.map((style) => `/api/plugins/${record.id}/ui${style}`);
}
