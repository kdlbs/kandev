import {
  IconBolt,
  IconLayoutGrid,
  IconMessageCircle,
  IconPlus,
  IconPuzzle,
  IconTerminal2,
} from "@tabler/icons-react";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { canvasHref } from "@/lib/api/domains/canvas-api";
import { AUTOMATIONS_HREF } from "@/components/runs/runs-view";
import type { Automation } from "@/lib/types/automation";
import type { ResolvedDestination, DestinationIcon } from "@/lib/navigation/types";
import type { SidebarShortcutTarget } from "./layout-types";

export type SidebarHostActionId = "new_task" | "quick_chat" | "quick_terminal";

export type ShortcutCatalogEntry = {
  target: SidebarShortcutTarget;
  label: string;
  icon?: DestinationIcon;
  href?: string;
  /** Original navigation section for registered plugin destinations. */
  section?: ResolvedDestination["section"];
  source?: "builtin" | "plugin" | "canvas" | "automation" | "host_action";
  available: boolean;
};

type AutomationCatalogItem = Pick<Automation, "id" | "workspace_id" | "name"> & Partial<Automation>;

type HostAction = {
  id: SidebarHostActionId;
  label: string;
  icon: DestinationIcon;
};

export type ShortcutCatalogOptions = {
  workspaceId?: string;
  destinations: ResolvedDestination[];
  canvases: Canvas[];
  automations: AutomationCatalogItem[];
  hostActions?: HostAction[];
  translate?: (key: string) => string;
};

const DEFAULT_HOST_ACTIONS: Array<{
  id: SidebarHostActionId;
  labelKey: string;
  icon: DestinationIcon;
}> = [
  { id: "new_task", labelKey: "sidebar:newTask", icon: IconPlus },
  { id: "quick_chat", labelKey: "sidebar:quickChat", icon: IconMessageCircle },
  { id: "quick_terminal", labelKey: "sidebar:quickTerminal", icon: IconTerminal2 },
];

function isEligibleCanvas(canvas: Canvas, workspaceId: string | undefined): boolean {
  return (
    (!workspaceId || canvas.workspace_id === workspaceId) &&
    canvas.scope_kind === "workspace" &&
    canvas.status === "active" &&
    canvas.active_release_status === "valid"
  );
}

export function buildShortcutCatalog({
  workspaceId,
  destinations,
  canvases,
  automations,
  hostActions,
  translate = (key) => key,
}: ShortcutCatalogOptions): ShortcutCatalogEntry[] {
  const destinationEntries = destinations.map((destination) => ({
    target: { kind: "destination" as const, id: destination.id },
    label: destination.label,
    icon: destination.icon,
    href: destination.href,
    section: destination.section,
    source: destination.source === "plugin" ? ("plugin" as const) : ("builtin" as const),
    available: true,
  }));
  const hostEntries = (
    hostActions ??
    DEFAULT_HOST_ACTIONS.map((action) => ({
      id: action.id,
      label: translate(action.labelKey),
      icon: action.icon,
    }))
  ).map((action) => ({
    target: { kind: "host_action" as const, id: action.id },
    label: action.label,
    icon: action.icon,
    source: "host_action" as const,
    available: true,
  }));
  const canvasEntries = canvases
    .filter((canvas) => isEligibleCanvas(canvas, workspaceId))
    .map((canvas) => ({
      target: { kind: "canvas" as const, id: canvas.id },
      label: canvas.title,
      icon: IconLayoutGrid,
      href: canvasHref(canvas.id),
      source: "canvas" as const,
      available: true,
    }));
  const automationEntries = automations
    .filter((automation) => !workspaceId || automation.workspace_id === workspaceId)
    .map((automation) => ({
      target: { kind: "automation" as const, id: automation.id },
      label: automation.name,
      icon: IconBolt,
      href: `${AUTOMATIONS_HREF}/${encodeURIComponent(automation.id)}`,
      source: "automation" as const,
      available: true,
    }));

  return [...destinationEntries, ...hostEntries, ...canvasEntries, ...automationEntries];
}

export function catalogEntryKey(entry: Pick<ShortcutCatalogEntry, "target">): string {
  return `${entry.target.kind}:${entry.target.id}`;
}

export const unavailableShortcutIcon = IconPuzzle;
