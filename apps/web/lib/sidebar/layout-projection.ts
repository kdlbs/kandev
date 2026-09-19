import {
  IconBolt,
  IconLayoutGrid,
  IconList,
  IconPlus,
  IconQuestionMark,
} from "@tabler/icons-react";
import type { DestinationIcon } from "@/lib/navigation/types";
import {
  DEFAULT_SIDEBAR_NODE_IDS,
  targetKey,
  type SidebarLayout,
  type SidebarLayoutNode,
  type SidebarShortcut,
} from "./layout-types";
import type { ShortcutCatalogEntry } from "./shortcut-catalog";
import { catalogEntryKey, unavailableShortcutIcon } from "./shortcut-catalog";

export type ProjectedShortcut = SidebarShortcut & {
  label: string;
  icon: DestinationIcon;
  href?: string;
  source: NonNullable<ShortcutCatalogEntry["source"]>;
  available: boolean;
};

export type ProjectedSidebarNode = Omit<SidebarLayoutNode, "shortcuts"> & {
  label: string;
  icon: DestinationIcon;
  shortcuts: ProjectedShortcut[];
};

export type SidebarLayoutProjection = {
  nodes: ProjectedSidebarNode[];
  protectedNodeIds: string[];
};

type ProjectionOptions = {
  unavailableLabel?: string;
  builtinLabels?: Record<string, string>;
};

const BUILTIN_ICONS: Record<string, DestinationIcon> = {
  home: IconList,
  new_task: IconPlus,
  automations: IconBolt,
  canvases: IconLayoutGrid,
  integrations: IconList,
};

function unavailableShortcut(shortcut: SidebarShortcut, label: string): ProjectedShortcut {
  return {
    ...shortcut,
    label,
    icon: unavailableShortcutIcon,
    source: shortcut.target.kind === "destination" ? "builtin" : shortcut.target.kind,
    available: false,
  };
}

function projectShortcut(
  shortcut: SidebarShortcut,
  catalog: Map<string, ShortcutCatalogEntry>,
  unavailableLabel: string,
): ProjectedShortcut {
  const entry = catalog.get(catalogEntryKey({ target: shortcut.target }));
  return entry
    ? {
        ...shortcut,
        ...entry,
        source: entry.source ?? "builtin",
        icon: entry.icon ?? unavailableShortcutIcon,
      }
    : unavailableShortcut(shortcut, unavailableLabel);
}

function pluginInsertionIndex(
  nodes: SidebarLayoutNode[],
  section: ShortcutCatalogEntry["section"],
): number {
  if (section === "plugins") {
    const newTaskIndex = nodes.findIndex((node) => node.destinationId === "new_task");
    return newTaskIndex >= 0 ? newTaskIndex + 1 : 0;
  }
  if (section === "integrations") {
    const integrationsIndex = nodes.findIndex((node) => node.destinationId === "integrations");
    return integrationsIndex >= 0 ? integrationsIndex + 1 : nodes.length;
  }
  return nodes.length;
}

/**
 * Adds registered plugin destinations to the editable node list. The section
 * comes from the navigation manifest, while the saved node controls visibility
 * and ordering after the user has moved it.
 */
export function materializeSidebarPluginNodes(
  layout: SidebarLayout,
  catalog: ShortcutCatalogEntry[],
): SidebarLayout {
  const pluginEntries = catalog.filter(
    (entry) => entry.source === "plugin" && entry.target.kind === "destination",
  );
  if (pluginEntries.length === 0) return layout;

  const nodes = layout.nodes.map((node) => {
    const entry = pluginEntries.find((item) => item.target.id === node.destinationId);
    return entry?.section && node.kind === "plugin"
      ? { ...node, pluginSection: entry.section }
      : node;
  });

  for (const entry of pluginEntries) {
    if (nodes.some((node) => node.id === entry.target.id)) continue;
    const index = pluginInsertionIndex(nodes, entry.section);
    nodes.splice(index, 0, {
      id: entry.target.id,
      kind: "plugin",
      visible: true,
      destinationId: entry.target.id,
      ...(entry.section ? { pluginSection: entry.section } : {}),
    });
  }
  return nodes.length === layout.nodes.length &&
    nodes.every((node, index) => node === layout.nodes[index])
    ? layout
    : { ...layout, nodes };
}

function projectNode(
  node: SidebarLayoutNode,
  catalog: Map<string, ShortcutCatalogEntry>,
  unavailableLabel: string,
  builtinLabels: Record<string, string>,
): ProjectedSidebarNode {
  const builtinIcon = node.destinationId ? BUILTIN_ICONS[node.destinationId] : undefined;
  const destination = node.destinationId
    ? catalog.get(`destination:${node.destinationId}`)
    : undefined;
  const label =
    node.name ??
    destination?.label ??
    (node.destinationId ? builtinLabels[node.destinationId] : undefined) ??
    node.id;
  const icon = destination?.icon ?? builtinIcon ?? IconQuestionMark;
  const shortcuts = (node.shortcuts ?? []).map((shortcut) =>
    projectShortcut(shortcut, catalog, unavailableLabel),
  );
  return { ...node, label, icon, shortcuts };
}

export function projectSidebarLayout(
  layout: SidebarLayout,
  catalog: ShortcutCatalogEntry[],
  options: ProjectionOptions = {},
): SidebarLayoutProjection {
  const byTarget = new Map(catalog.map((entry) => [catalogEntryKey(entry), entry]));
  const nodes = materializeSidebarPluginNodes(layout, catalog).nodes.map((node) =>
    projectNode(node, byTarget, options.unavailableLabel ?? node.id, options.builtinLabels ?? {}),
  );
  return {
    nodes,
    protectedNodeIds: ["tasks", "inbox", "needs-you-inbox"],
  };
}

export function hasShortcut(layout: SidebarLayout, target: SidebarShortcut["target"]): boolean {
  return layout.nodes.some((node) =>
    node.shortcuts?.some((shortcut) => targetKey(shortcut.target) === targetKey(target)),
  );
}

export function canonicalSidebarNodeIds(): readonly string[] {
  return DEFAULT_SIDEBAR_NODE_IDS;
}
