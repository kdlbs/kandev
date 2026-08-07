"use client";

import { useMemo } from "react";

import { useAppStore } from "@/components/state-provider";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { EXECUTORS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/executors";
import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";
import { isTreeSettingsMenuMode, type SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import {
  buildAgentsBranch,
  buildBranchRoot,
  buildExecutorsBranch,
  buildWorkspacesBranch,
  type SettingsMenuNode,
} from "./settings-menu-branches";
import { SETTINGS_MENU_SECTIONS, type SettingsMenuItem } from "./settings-menu-sections";

/** Branch root per menu row href. Rows with no entry render as plain leaves. */
export type SettingsMenuBranches = Record<string, SettingsMenuNode>;

const NO_BRANCHES: SettingsMenuBranches = {};

const MENU_ITEMS_BY_HREF: ReadonlyMap<string, SettingsMenuItem> = new Map(
  SETTINGS_MENU_SECTIONS.flatMap((section) =>
    section.items.map((item) => [item.href, item] as const),
  ),
);

/**
 * The live branches for the current mode, keyed by the row they hang under.
 *
 * `flat` returns a shared empty object, so the menu does no branch work at all
 * and every row renders exactly as it did before the tree modes existed — the
 * default path stays the cheap one.
 *
 * The three record lists come from the settings bootstrap
 * (`SettingsRouteBootstrap`), which is also what fills the rows' count badges,
 * so a branch never lists records its own row's badge disagrees with.
 */
export function useSettingsMenuBranches(mode: SettingsMenuMode): SettingsMenuBranches {
  const isTree = isTreeSettingsMenuMode(mode);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const agents = useAppStore((s) => s.settingsAgents.items);
  const executors = useAppStore((s) => s.executors.items);

  return useMemo(() => {
    if (!isTree) return NO_BRANCHES;
    return {
      ...branchEntry(WORKSPACES_SETTINGS_HREF, buildWorkspacesBranch(workspaces)),
      ...branchEntry(AGENTS_SETTINGS_HREF, buildAgentsBranch(agents)),
      ...branchEntry(EXECUTORS_SETTINGS_HREF, buildExecutorsBranch(executors)),
    };
  }, [isTree, workspaces, agents, executors]);
}

/**
 * A branch keyed by its row, or nothing when the row has no records yet. An
 * empty branch would render a chevron that opens onto an empty list, which
 * reads as a loading failure rather than as "you have no workspaces".
 */
function branchEntry(href: string, children: SettingsMenuNode[]): SettingsMenuBranches {
  const item = MENU_ITEMS_BY_HREF.get(href);
  if (!item || children.length === 0) return {};
  return { [href]: buildBranchRoot(item, children) };
}

/** Every branch in one forest — what expansion and active state are keyed over. */
export function useSettingsMenuForest(branches: SettingsMenuBranches): SettingsMenuNode[] {
  return useMemo(() => Object.values(branches), [branches]);
}
