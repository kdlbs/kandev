import type {
  SidebarLayoutApi,
  SidebarLayoutNodeApi,
  SidebarShortcutApi,
  SidebarShortcutTargetApi,
} from "@/lib/types/http-user-settings";
import type { NavSection } from "@/lib/navigation/types";
import { BUILTIN_LAYOUT_NODES, type BuiltinLayoutNodeId } from "./builtin-layout-nodes";

export const SIDEBAR_LAYOUT_VERSION = 1;
export const SIDEBAR_LAYOUT_LIMITS = {
  groups: 20,
  shortcutsPerGroup: 20,
  shortcuts: 100,
  sectionNameCodePoints: 60,
} as const;

export type SidebarShortcutTargetKind = SidebarShortcutTargetApi["kind"];
export type SidebarShortcutTarget = SidebarShortcutTargetApi;
export type SidebarShortcut = SidebarShortcutApi;
export type SidebarLayoutNodeKind = SidebarLayoutNodeApi["kind"];

export type SidebarLayoutNode = {
  id: string;
  kind: SidebarLayoutNodeKind;
  visible: boolean;
  destinationId?: string;
  /** Frontend metadata used to retain a registered destination's source section. */
  pluginSection?: NavSection;
  name?: string;
  shortcuts?: SidebarShortcut[];
};

export type SidebarLayout = {
  navigationHeight?: number;
  navigationExpanded?: boolean;
  version: number;
  revision: number;
  nodes: SidebarLayoutNode[];
  unsupportedVersion?: boolean;
};

export const DEFAULT_SIDEBAR_NODE_IDS = BUILTIN_LAYOUT_NODES.map((node) => node.id);

export type SidebarBuiltinNodeId = BuiltinLayoutNodeId;

export function defaultSidebarLayout(): SidebarLayout {
  return {
    version: SIDEBAR_LAYOUT_VERSION,
    revision: 0,
    nodes: BUILTIN_LAYOUT_NODES.map(({ id, destinationId }) => ({
      id,
      kind: "builtin",
      visible: true,
      destinationId,
    })),
  };
}

export function targetKey(target: SidebarShortcutTarget): string {
  return `${target.kind}:${target.id}`;
}

export function fromApiSidebarLayout(value: SidebarLayoutApi | null | undefined): SidebarLayout {
  if (!value) return defaultSidebarLayout();
  if (value.version !== SIDEBAR_LAYOUT_VERSION) {
    return {
      ...defaultSidebarLayout(),
      revision: Math.max(0, value.revision),
      unsupportedVersion: true,
    };
  }
  return {
    version: value.version,
    revision: Math.max(0, value.revision),
    ...(value.navigation_height !== undefined ? { navigationHeight: value.navigation_height } : {}),
    ...(value.navigation_expanded !== undefined
      ? { navigationExpanded: value.navigation_expanded }
      : {}),
    ...(value.unsupported_version ? { unsupportedVersion: true } : {}),
    nodes: materializeSidebarBuiltinNodes(
      value.nodes.map((node) => ({
        id: node.id,
        kind: node.kind,
        visible: node.visible,
        ...(node.destination_id !== undefined ? { destinationId: node.destination_id } : {}),
        ...(node.name !== undefined ? { name: node.name } : {}),
        ...(node.shortcuts
          ? {
              shortcuts: node.shortcuts.map((shortcut) => ({
                id: shortcut.id,
                target: { ...shortcut.target },
              })),
            }
          : {}),
      })),
    ),
  };
}

export function materializeInboxNodes(nodes: SidebarLayoutNode[]): SidebarLayoutNode[] {
  const missing = defaultSidebarLayout().nodes.filter(
    (node) =>
      (node.destinationId === "inbox" || node.destinationId === "needs_you_inbox") &&
      !nodes.some((saved) => saved.destinationId === node.destinationId),
  );
  if (!missing.length) return nodes;
  const result = [...nodes];
  const homeIndex = result.findIndex((node) => node.destinationId === "home");
  result.splice(homeIndex >= 0 ? homeIndex + 1 : result.length, 0, ...missing);
  return result;
}

export function materializeSidebarBuiltinNodes(nodes: SidebarLayoutNode[]): SidebarLayoutNode[] {
  nodes = materializeInboxNodes(nodes);
  if (nodes.some((node) => node.kind === "builtin" && node.destinationId === "coordinators")) {
    return nodes;
  }
  const coordinator = BUILTIN_LAYOUT_NODES.find((node) => node.destinationId === "coordinators");
  if (!coordinator) return nodes;
  const usedIds = new Set(nodes.map((node) => node.id));
  let id: string = coordinator.id;
  for (let suffix = 1; usedIds.has(id); suffix += 1) {
    id = `${coordinator.id}-${suffix}`;
  }
  const entry: SidebarLayoutNode = {
    id,
    kind: "builtin",
    visible: true,
    destinationId: coordinator.destinationId,
  };
  const automationsIndex = nodes.findIndex((node) => node.destinationId === "automations");
  const index = automationsIndex < 0 ? nodes.length : automationsIndex;
  return [...nodes.slice(0, index), entry, ...nodes.slice(index)];
}

export function toApiSidebarLayout(value: SidebarLayout): SidebarLayoutApi {
  return {
    version: value.version,
    revision: value.revision,
    ...(value.navigationHeight !== undefined ? { navigation_height: value.navigationHeight } : {}),
    ...(value.navigationExpanded !== undefined
      ? { navigation_expanded: value.navigationExpanded }
      : {}),
    nodes: value.nodes.map((node) => ({
      id: node.id,
      kind: node.kind,
      visible: node.visible,
      ...(node.destinationId !== undefined ? { destination_id: node.destinationId } : {}),
      ...(node.name !== undefined ? { name: node.name } : {}),
      ...(node.shortcuts
        ? {
            shortcuts: node.shortcuts.map((shortcut) => ({
              id: shortcut.id,
              target: { ...shortcut.target },
            })),
          }
        : {}),
    })),
  };
}
