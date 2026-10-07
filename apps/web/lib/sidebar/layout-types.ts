import type {
  SidebarLayoutApi,
  SidebarLayoutNodeApi,
  SidebarShortcutApi,
  SidebarShortcutTargetApi,
} from "@/lib/types/http-user-settings";
import type { NavSection } from "@/lib/navigation/types";

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

export const DEFAULT_SIDEBAR_NODE_IDS = [
  "new-task",
  "home",
  "inbox",
  "needs-you-inbox",
  "automations",
  "canvases",
  "integrations",
] as const;

export type SidebarBuiltinNodeId = (typeof DEFAULT_SIDEBAR_NODE_IDS)[number];

export function defaultSidebarLayout(): SidebarLayout {
  return {
    version: SIDEBAR_LAYOUT_VERSION,
    revision: 0,
    nodes: [
      {
        id: "new-task",
        kind: "builtin",
        visible: true,
        destinationId: "new_task",
      },
      {
        id: "home",
        kind: "builtin",
        visible: true,
        destinationId: "home",
      },
      { id: "inbox", kind: "builtin", visible: true, destinationId: "inbox" },
      { id: "needs-you-inbox", kind: "builtin", visible: true, destinationId: "needs_you_inbox" },
      {
        id: "automations",
        kind: "builtin",
        visible: true,
        destinationId: "automations",
      },
      {
        id: "canvases",
        kind: "builtin",
        visible: true,
        destinationId: "canvases",
      },
      {
        id: "integrations",
        kind: "builtin",
        visible: true,
        destinationId: "integrations",
      },
    ],
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
    nodes: materializeInboxNodes(
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
