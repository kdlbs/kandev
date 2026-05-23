import { useEffect, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";

const MINIMIZED_ATTR = "data-kandev-minimized";
/** Minimum splitview size we request when minimizing — the splitview clamps to the
 *  group's actual minimum (tab-strip height / minimum group width) so this can be 0. */
const MIN_SIZE = 0;

type PriorSize = { width: number; height: number };

/** Apply a single group's minimized state: data-attribute + splitview size shrink/restore. */
function applyGroupMinimized(
  api: DockviewApi,
  groupId: string,
  minimized: boolean,
  priorSizes: Map<string, PriorSize>,
): void {
  const group = api.groups.find((g) => g.id === groupId);
  if (!group) return;
  const next = minimized ? "true" : "false";
  if (group.element.getAttribute(MINIMIZED_ATTR) !== next) {
    group.element.setAttribute(MINIMIZED_ATTR, next);
  }
  if (minimized) {
    if (!priorSizes.has(groupId)) {
      priorSizes.set(groupId, { width: group.api.width, height: group.api.height });
    }
    try {
      group.api.setSize({ width: MIN_SIZE, height: MIN_SIZE });
    } catch {
      /* setSize may throw if the group is being torn down */
    }
  } else {
    const prior = priorSizes.get(groupId);
    if (prior) {
      try {
        group.api.setSize({ width: prior.width, height: prior.height });
      } catch {
        /* same */
      }
      priorSizes.delete(groupId);
    }
  }
}

/**
 * Keep the DOM and splitview sizes in sync with the store's `minimizedGroupIds`.
 *
 * - On set change: walk every group, set the `data-kandev-minimized` attribute,
 *   and call `group.api.setSize` to either shrink (clamps to splitview minimum)
 *   or restore the prior recorded size.
 * - On `onDidAddPanel` into a minimized group: auto-restore (user is trying to
 *   add something they want to see).
 * - On `onDidRemoveGroup`: evict the group from `minimizedGroupIds` and prior-sizes.
 * - On `onDidAddGroup`: new groups default to not-minimized (no-op — they don't
 *   appear in the set).
 */
export function useMinimizedGroupSync(api: DockviewApi | null): void {
  const priorSizesRef = useRef<Map<string, PriorSize>>(new Map());

  useEffect(() => {
    if (!api) return;
    const priorSizes = priorSizesRef.current;

    const sync = () => {
      const ids = useDockviewStore.getState().minimizedGroupIds;
      for (const group of api.groups) {
        applyGroupMinimized(api, group.id, ids.has(group.id), priorSizes);
      }
    };

    sync();

    const unsubscribe = useDockviewStore.subscribe((state, prev) => {
      if (state.minimizedGroupIds === prev.minimizedGroupIds) return;
      sync();
    });

    const d1 = api.onDidAddGroup(() => sync());
    const d2 = api.onDidRemoveGroup((group) => {
      const ids = useDockviewStore.getState().minimizedGroupIds;
      priorSizes.delete(group.id);
      if (ids.has(group.id)) {
        const next = new Set(ids);
        next.delete(group.id);
        useDockviewStore.setState({ minimizedGroupIds: next });
      }
    });
    const d3 = api.onDidAddPanel((panel) => {
      const groupId = panel.group?.id;
      if (!groupId) return;
      const ids = useDockviewStore.getState().minimizedGroupIds;
      if (!ids.has(groupId)) return;
      const next = new Set(ids);
      next.delete(groupId);
      useDockviewStore.setState({ minimizedGroupIds: next });
    });

    return () => {
      unsubscribe();
      d1.dispose();
      d2.dispose();
      d3.dispose();
    };
  }, [api]);
}
