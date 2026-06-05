/**
 * Dockview components whose panel content is tied to one task environment.
 *
 * These panels carry env-specific runtime state such as file paths, iframes,
 * editor buffers, git history, or PR metadata. They may persist inside a
 * saved per-env layout, but must not be restored from a no-env/global fallback
 * while a newly selected task is still preparing its environment.
 */
export const ENV_SCOPED_DOCKVIEW_COMPONENTS = new Set([
  "file-editor",
  "browser",
  "vscode",
  "commit-detail",
  "diff-viewer",
  "pr-detail",
]);

export function isEnvScopedDockviewComponent(component: string | null | undefined): boolean {
  return !!component && ENV_SCOPED_DOCKVIEW_COMPONENTS.has(component);
}
