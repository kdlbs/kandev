/**
 * Component names the desktop workbench can instantiate.
 *
 * Static rather than a runtime registry populated by the renderer module:
 * `components/task/dockview-desktop-layout.tsx` is loaded through
 * `dynamic(() => import(...), { ssr: false })`, and the settings route that
 * validates saved layout profiles never imports it. A registry populated at
 * that module's load would be empty there, so profile normalization would
 * behave differently depending on whether the user had visited a task first.
 *
 * The list must stay equal to that renderer's component map, including the
 * legacy aliases: a stored panel whose component is missing here is dropped
 * when a layout is applied.
 */
export const RENDERABLE_COMPONENT_NAMES = [
  "chat",
  "diff-viewer",
  "file-editor",
  "commit-detail",
  "changes",
  "files",
  "terminal",
  "browser",
  "vscode",
  "plan",
  "todos",
  "background-work",
  "pr-detail",
  "mr-detail",
  "review-detail",
  "plugin-panel",
  "canvas",
  // Backwards compat aliases for saved layouts
  "diff-files",
  "all-files",
] as const;

/** Set form of `RENDERABLE_COMPONENT_NAMES` for the layout filters. */
export const RENDERABLE_COMPONENTS: ReadonlySet<string> = new Set(RENDERABLE_COMPONENT_NAMES);

/** True when the desktop renderer can instantiate a panel with this component. */
export function isRenderableComponent(component: string | undefined): boolean {
  return Boolean(component && RENDERABLE_COMPONENTS.has(component));
}
