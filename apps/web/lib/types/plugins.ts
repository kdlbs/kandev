/**
 * TS mirror of the plugin registration types in
 * apps/backend/internal/plugins/manifest/manifest.go and
 * apps/backend/internal/plugins/store/store.go. Field names are snake_case
 * to match the backend JSON tags verbatim (no camelCase transform layer —
 * see lib/api/client.ts fetchJson).
 */

export type PluginStatus = "registered" | "active" | "error" | "disabled" | "uninstalled";

export interface PluginCapabilities {
  events?: string[];
  api_read?: string[];
  api_write?: string[];
  state?: boolean;
  secrets?: boolean;
}

export interface PluginWebhook {
  key: string;
  description?: string;
  method?: string;
}

export interface PluginUIPage {
  key: string;
  title: string;
  path: string;
  surface: string;
}

export interface PluginUISection {
  pages?: PluginUIPage[];
  bundle?: string;
  styles?: string[];
}

/**
 * A stored, installed plugin, as returned by GET/PATCH /api/plugins/... and
 * by POST /api/plugins/install. Installation is package-based (tarball URL
 * or upload) — there is no base_url/endpoints registration and no
 * api_key/webhook_secret credential pair (see docs/plans/plugins/GRPC-CONTRACT.md
 * §6-§7). `ui.bundle` is a package-relative path like "ui/bundle.js"; the
 * bundle is always served by kandev from the extracted package dir at
 * /api/plugins/{id}/bundle regardless of that path.
 */
export interface PluginRecord {
  id: string;
  api_version: number;
  version: string;
  display_name: string;
  description: string;
  author: string;
  categories: string[];
  capabilities: PluginCapabilities;
  webhooks?: PluginWebhook[];
  config_schema?: Record<string, unknown>;
  ui?: PluginUISection;
  status: PluginStatus;
  /** Absolute path the package was extracted to: ~/.kandev/plugins/<id>/<version>/ */
  install_path: string;
  /** false when checksums.txt.sig was missing/unverifiable at install time. */
  signed: boolean;
  installed_at: string;
  /** Crash-restart attempts since install (health-check backoff counter). */
  restart_count: number;
  last_health_check?: string | null;
}

/**
 * One entry of SyncResult.errors: a filesystem path the sync scan rejected
 * (or skipped), plus a human-readable reason. Mirrors
 * apps/backend/internal/plugins/dto.go's SyncError.
 */
export interface SyncError {
  path: string;
  reason: string;
}

/**
 * The response of POST /api/plugins/sync: what the filesystem scan under
 * the plugins directory found and did this run. Mirrors
 * apps/backend/internal/plugins/dto.go's SyncResult.
 */
export interface SyncResult {
  /** Plugin ids of directory sideloads registered this run (always `disabled`). */
  added: string[];
  /** Plugin ids of dropped *.tar.gz packages installed this run. */
  installed: string[];
  /** Plugin ids whose install path no longer exists on disk (now `error`). */
  missing: string[];
  errors: SyncError[];
}
