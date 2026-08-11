"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { useAppStore } from "@/components/state-provider";
import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import {
  findActiveNodePath,
  findNodeKeyPath,
  findSubtreeKeys,
  type SettingsMenuNode,
} from "./settings-menu-branches";

export type SettingsMenuExpansion = {
  isExpanded: (key: string) => boolean;
  toggle: (key: string) => void;
  /**
   * The deepest node owning the current route, or null when no branch does.
   * Null is also what tells a menu *row* it may mark itself active: exactly one
   * row in the menu carries `data-active`, and a node that claims the route
   * takes it from the row above.
   */
  activeKey: string | null;
};

/** The two hooks below own open/closed state; `activeKey` is derived once above. */
type ExpansionControls = Omit<SettingsMenuExpansion, "activeKey">;

const KEY_SEPARATOR = ">";

/**
 * Which branches of the settings menu are open, per mode.
 *
 * - `accordion` keeps **one** open path. Opening a branch closes its siblings,
 *   and navigating re-derives the path from the route, so the menu always shows
 *   the way to the current page and nothing else. State is per mount and
 *   deliberately not persisted: the route already determines it.
 * - `persistent` keeps a set of open keys that survives navigation *and*
 *   reload. Navigating only ever adds the current page's ancestors — it never
 *   closes a branch the user opened, which is the whole difference from
 *   accordion.
 * - `flat` renders no branches, so nothing here is ever consulted.
 *
 * Both modes are computed every render regardless of the active one: hooks
 * cannot be called conditionally, and the unused half costs a walk over a menu
 * whose size is bounded by the workspace, agent and executor counts.
 */
export function useSettingsMenuExpansion(
  mode: SettingsMenuMode,
  nodes: ReadonlyArray<SettingsMenuNode>,
  pathname: string,
): SettingsMenuExpansion {
  // Joined into a string rather than compared by identity: `nodes` is rebuilt
  // from the store every render, so the path array is never referentially
  // stable and would re-fire every effect that depended on it.
  const activePath = findActiveNodePath(nodes, pathname);
  const activePathKey = activePath.join(KEY_SEPARATOR);
  const activeKey = activePath.length > 0 ? activePath[activePath.length - 1] : null;

  const accordion = useAccordionExpansion(nodes, activePathKey);
  const persistent = usePersistentExpansion(nodes, activePathKey);
  const expansion = mode === "persistent" ? persistent : accordion;

  return { ...expansion, activeKey };
}

function splitPath(activePathKey: string): string[] {
  return activePathKey ? activePathKey.split(KEY_SEPARATOR) : [];
}

function useAccordionExpansion(
  nodes: ReadonlyArray<SettingsMenuNode>,
  activePathKey: string,
): ExpansionControls {
  const [openPath, setOpenPath] = useState<string[]>(() => splitPath(activePathKey));

  // Re-sync when navigation lands somewhere else, so the open path always
  // reflects the current page (a route under no branch collapses everything).
  useEffect(() => {
    setOpenPath(splitPath(activePathKey));
  }, [activePathKey]);

  const isExpanded = useCallback((key: string) => openPath.includes(key), [openPath]);

  const toggle = useCallback(
    (key: string) => {
      setOpenPath((current) => {
        const index = current.indexOf(key);
        // Closing a branch closes whatever it was holding open beneath it;
        // opening one replaces the path, which is what closes its siblings.
        return index === -1 ? findNodeKeyPath(nodes, key) : current.slice(0, index);
      });
    },
    [nodes],
  );

  return { isExpanded, toggle };
}

function usePersistentExpansion(
  nodes: ReadonlyArray<SettingsMenuNode>,
  activePathKey: string,
): ExpansionControls {
  const expandedKeys = useAppStore((s) => s.settingsMenu.expandedKeys);
  const setExpandedKeys = useAppStore((s) => s.setSettingsMenuExpandedKeys);
  const expanded = useMemo(() => new Set(expandedKeys), [expandedKeys]);

  // Reveal the current page without disturbing anything already open. Guarded
  // on "adds something new" so a route inside an open branch does not write.
  useEffect(() => {
    const path = splitPath(activePathKey);
    if (path.length === 0 || path.every((key) => expanded.has(key))) return;
    setExpandedKeys([...new Set([...expanded, ...path])]);
    // Intentionally keyed on the route alone: `expanded` is derived from the
    // keys this effect writes, so listing it would re-run the merge on its own
    // output. The guard above re-reads the live set either way.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activePathKey]);

  const isExpanded = useCallback((key: string) => expanded.has(key), [expanded]);

  const toggle = useCallback(
    (key: string) => {
      const next = new Set(expanded);
      if (next.has(key)) {
        for (const descendant of findSubtreeKeys(nodes, key)) next.delete(descendant);
      } else {
        for (const ancestor of findNodeKeyPath(nodes, key)) next.add(ancestor);
      }
      setExpandedKeys([...next]);
    },
    [expanded, nodes, setExpandedKeys],
  );

  return { isExpanded, toggle };
}
