import type { DockviewApi } from "dockview-react";

/**
 * Reorder session panels within their shared tab group to match
 * `currentSessionIds` — the step-flow-sorted order used for tab labels and
 * ranks. `ensureSessionPanel`/`ensureSiblingPanels` (in dockview-session-tabs)
 * only append newly-created panels; they never move a panel that already
 * existed. Without this, an existing "Work #2" panel could keep sitting ahead
 * of a freshly created "Spec #1" panel after a workflow-step move reorders
 * the session list.
 *
 * Only panels sharing `activePanel`'s group are reordered, matching
 * `ensureSessionTabPrecedesNonSessionTabs`'s scope — a panel the user
 * manually dragged into a different group is left alone.
 */
export function reconcileSessionPanelOrder(
  api: DockviewApi,
  currentSessionIds: string[],
  activePanel: ReturnType<DockviewApi["getPanel"]>,
): void {
  if (!activePanel) return;
  const group = activePanel.group;
  // Index within the *co-located* subset, not the full session list — a
  // sibling panel the user dragged into a different group must not shift
  // this group's panels just because it occupies an earlier slot globally.
  let index = 0;
  for (const sessionId of currentSessionIds) {
    const panel = api.getPanel(`session:${sessionId}`);
    if (!panel || panel.group !== group) continue;
    const currentIndex = group.panels?.indexOf(panel) ?? -1;
    if (currentIndex !== index) {
      panel.api.moveTo({
        group,
        position: "center",
        index,
        skipSetActive: true,
      });
    }
    index += 1;
  }
}
