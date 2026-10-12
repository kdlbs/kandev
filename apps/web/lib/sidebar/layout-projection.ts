import { IconQuestionMark } from "@tabler/icons-react";
import type { DestinationIcon } from "@/lib/navigation/types";
import {
  builtinLayoutNodeForDestination,
  defaultBuiltinLayoutContext,
  isBuiltinLayoutNodeEligible,
  type BuiltinLayoutContext,
} from "./builtin-layout-nodes";
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
  pluginItemId?: ShortcutCatalogEntry["pluginItemId"];
  shortcuts: ProjectedShortcut[];
  available?: boolean;
};

export type SidebarLayoutProjection = {
  nodes: ProjectedSidebarNode[];
  protectedNodeIds: string[];
};

type ProjectionOptions = {
  unavailableLabel?: string;
  builtinLabels?: Record<string, string>;
  builtinContext?: BuiltinLayoutContext;
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
  builtinContext: BuiltinLayoutContext,
): ProjectedShortcut {
  const builtin =
    shortcut.target.kind === "destination"
      ? builtinLayoutNodeForDestination(shortcut.target.id)
      : undefined;
  if (builtin && !isBuiltinLayoutNodeEligible(builtin, builtinContext)) {
    return unavailableShortcut(shortcut, unavailableLabel);
  }
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

  for (const entry of [...pluginEntries].reverse()) {
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

function projectBuiltinNodePresentation(
  node: SidebarLayoutNode,
  unavailableLabel: string,
  builtinLabels: Record<string, string>,
  builtinContext: BuiltinLayoutContext,
): Pick<ProjectedSidebarNode, "label" | "icon" | "pluginItemId" | "available"> {
  const builtin = builtinLayoutNodeForDestination(node.destinationId);
  const label =
    node.name ?? builtinLabels[node.destinationId ?? ""] ?? builtin?.labelKey ?? unavailableLabel;
  const available = Boolean(builtin && isBuiltinLayoutNodeEligible(builtin, builtinContext));
  return {
    label,
    icon: builtin?.icon ?? unavailableShortcutIcon,
    available,
  };
}

function projectPluginNodePresentation(
  node: SidebarLayoutNode,
  destination: ShortcutCatalogEntry | undefined,
  unavailableLabel: string,
  builtinLabels: Record<string, string>,
): Pick<ProjectedSidebarNode, "label" | "icon" | "pluginItemId" | "available"> {
  if (!destination || destination.available === false) {
    return {
      label: destination?.label ?? unavailableLabel,
      icon: unavailableShortcutIcon,
      available: false,
    };
  }
  return {
    label:
      node.name ??
      destination.label ??
      (node.destinationId ? builtinLabels[node.destinationId] : undefined) ??
      node.id,
    icon: destination.icon ?? IconQuestionMark,
    ...(destination?.pluginItemId ? { pluginItemId: destination.pluginItemId } : {}),
    available: true,
  };
}

function projectNodePresentation(
  node: SidebarLayoutNode,
  catalog: Map<string, ShortcutCatalogEntry>,
  unavailableLabel: string,
  builtinLabels: Record<string, string>,
  builtinContext: BuiltinLayoutContext,
): Pick<ProjectedSidebarNode, "label" | "icon" | "pluginItemId" | "available"> {
  if (node.kind === "builtin") {
    return projectBuiltinNodePresentation(node, unavailableLabel, builtinLabels, builtinContext);
  }
  const destination = node.destinationId
    ? catalog.get(`destination:${node.destinationId}`)
    : undefined;
  if (node.kind === "plugin") {
    return projectPluginNodePresentation(node, destination, unavailableLabel, builtinLabels);
  }
  return {
    label:
      node.name ??
      destination?.label ??
      (node.destinationId ? builtinLabels[node.destinationId] : undefined) ??
      node.id,
    icon: destination?.icon ?? IconQuestionMark,
    ...(destination?.pluginItemId ? { pluginItemId: destination.pluginItemId } : {}),
    available: true,
  };
}

function projectNode(
  node: SidebarLayoutNode,
  catalog: Map<string, ShortcutCatalogEntry>,
  unavailableLabel: string,
  builtinLabels: Record<string, string>,
  builtinContext: BuiltinLayoutContext,
): ProjectedSidebarNode {
  const presentation = projectNodePresentation(
    node,
    catalog,
    unavailableLabel,
    builtinLabels,
    builtinContext,
  );
  const shortcuts = (node.shortcuts ?? []).map((shortcut) =>
    projectShortcut(shortcut, catalog, unavailableLabel, builtinContext),
  );
  return {
    ...node,
    ...presentation,
    shortcuts,
  };
}

export function projectSidebarLayout(
  layout: SidebarLayout,
  catalog: ShortcutCatalogEntry[],
  options: ProjectionOptions = {},
): SidebarLayoutProjection {
  const byTarget = new Map(catalog.map((entry) => [catalogEntryKey(entry), entry]));
  const builtinContext = options.builtinContext ?? defaultBuiltinLayoutContext();
  const nodes = materializeSidebarPluginNodes(layout, catalog).nodes.map((node) =>
    projectNode(
      node,
      byTarget,
      options.unavailableLabel ?? node.id,
      options.builtinLabels ?? {},
      builtinContext,
    ),
  );
  return {
    nodes,
    protectedNodeIds: ["tasks"],
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
