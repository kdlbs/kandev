import type { ProjectedSidebarNode } from "./layout-projection";
import type { ShortcutCatalogEntry } from "./shortcut-catalog";
import type { ResolvedDestination } from "@/lib/navigation/types";
import type { SidebarLayoutNode } from "./layout-types";

export function isIntegrationPlugin(node: SidebarLayoutNode): boolean {
  return node.kind === "plugin" && node.pluginSection === "integrations";
}

/** Keeps the portable flat node list separate from the rendered hierarchy. */
export function integrationPluginLayout<T extends SidebarLayoutNode>(nodes: T[]) {
  const children = nodes.filter(isIntegrationPlugin);
  const topLevel = nodes.filter((node) => !isIntegrationPlugin(node));
  return { children, topLevel };
}

export function sidebarNodeSiblings<T extends SidebarLayoutNode>(nodes: T[], nodeId: string): T[] {
  const node = nodes.find((item) => item.id === nodeId);
  return nodes.filter((item) => isIntegrationPlugin(item) === (node && isIntegrationPlugin(node)));
}

/** Display integration choices beneath their parent without changing stored order. */
export function sidebarEditorNodes<T extends SidebarLayoutNode>(nodes: T[]): T[] {
  const { children, topLevel } = integrationPluginLayout(nodes);
  const anchor = topLevel.findIndex((node) => node.destinationId === "integrations");
  return anchor < 0
    ? [...topLevel, ...children]
    : [...topLevel.slice(0, anchor + 1), ...children, ...topLevel.slice(anchor + 1)];
}

export function integrationPluginDestinations(
  nodes: ProjectedSidebarNode[],
  catalog: ShortcutCatalogEntry[],
): ResolvedDestination[] {
  return nodes
    .filter((node) => isIntegrationPlugin(node) && node.visible && node.available !== false)
    .flatMap((node) => {
      const entry = catalog.find(
        (item) => item.target.kind === "destination" && item.target.id === node.destinationId,
      );
      return entry?.href && entry.available
        ? [
            {
              id: node.destinationId!,
              label: node.label,
              icon: node.icon,
              href: entry.href,
              section: "integrations" as const,
              source: "plugin" as const,
              pluginItemId: node.pluginItemId,
            },
          ]
        : [];
    });
}
