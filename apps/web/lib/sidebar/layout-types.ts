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
  version: number;
  revision: number;
  nodes: SidebarLayoutNode[];
};

export const DEFAULT_SIDEBAR_NODE_IDS = [
  "home",
  "new-task",
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
        id: "home",
        kind: "builtin",
        visible: true,
        destinationId: "home",
      },
      {
        id: "new-task",
        kind: "builtin",
        visible: true,
        destinationId: "new_task",
      },
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
  if (!value || value.version !== SIDEBAR_LAYOUT_VERSION) return defaultSidebarLayout();
  return {
    version: value.version,
    revision: Math.max(0, value.revision),
    nodes: value.nodes.map((node) => ({
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
  };
}

export function toApiSidebarLayout(value: SidebarLayout): SidebarLayoutApi {
  return {
    version: value.version,
    revision: value.revision,
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
