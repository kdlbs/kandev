import { IconBolt, IconInbox, IconLayoutGrid, IconList, IconPlus } from "@tabler/icons-react";
import type { DestinationIcon } from "@/lib/navigation/types";
import { CoordinatorIcon } from "@/lib/coordinator/icon";

export type BuiltinLayoutContext = {
  hasWorkspace: boolean;
  inOffice: boolean;
  features: {
    coordinator: boolean;
    canvases: boolean;
    needsYouInbox: boolean;
  };
};

export type BuiltinLayoutNodeDescriptor = {
  id: string;
  destinationId: string;
  labelKey: string;
  icon: DestinationIcon;
  isEligible: (context: BuiltinLayoutContext) => boolean;
};

export const BUILTIN_LAYOUT_NODES = [
  {
    id: "new-task",
    destinationId: "new_task",
    labelKey: "sidebar:newTask",
    icon: IconPlus,
    isEligible: () => true,
  },
  {
    id: "home",
    destinationId: "home",
    labelKey: "sidebar:home",
    icon: IconList,
    isEligible: () => true,
  },
  {
    id: "inbox",
    destinationId: "inbox",
    labelKey: "sidebar:inbox",
    icon: IconInbox,
    isEligible: (context: BuiltinLayoutContext) => context.inOffice,
  },
  {
    id: "needs-you-inbox",
    destinationId: "needs_you_inbox",
    labelKey: "sidebar:inbox",
    icon: IconInbox,
    isEligible: (context: BuiltinLayoutContext) => context.features.needsYouInbox,
  },
  {
    id: "coordinators",
    destinationId: "coordinators",
    labelKey: "coordinator:sidebarSectionLabel",
    icon: CoordinatorIcon,
    isEligible: (context: BuiltinLayoutContext) =>
      context.hasWorkspace && context.features.coordinator,
  },
  {
    id: "automations",
    destinationId: "automations",
    labelKey: "automations:automations",
    icon: IconBolt,
    isEligible: (context: BuiltinLayoutContext) => context.hasWorkspace && !context.inOffice,
  },
  {
    id: "canvases",
    destinationId: "canvases",
    labelKey: "canvases:canvases",
    icon: IconLayoutGrid,
    isEligible: (context: BuiltinLayoutContext) =>
      context.hasWorkspace && context.features.canvases && !context.inOffice,
  },
  {
    id: "integrations",
    destinationId: "integrations",
    labelKey: "common:integrations",
    icon: IconList,
    isEligible: (context: BuiltinLayoutContext) => context.hasWorkspace && !context.inOffice,
  },
] as const satisfies readonly BuiltinLayoutNodeDescriptor[];

export type BuiltinLayoutNodeId = (typeof BUILTIN_LAYOUT_NODES)[number]["id"];

export function builtinLayoutNodeForDestination(
  destinationId: string | undefined,
): BuiltinLayoutNodeDescriptor | undefined {
  return destinationId
    ? BUILTIN_LAYOUT_NODES.find((node) => node.destinationId === destinationId)
    : undefined;
}

export function builtinLayoutNodeLabelKey(
  node: BuiltinLayoutNodeDescriptor,
  context: BuiltinLayoutContext,
): string {
  if (node.destinationId === "needs_you_inbox" && context.inOffice) {
    return "sidebar:needsYouInbox";
  }
  return node.labelKey;
}

export function isBuiltinLayoutNodeEligible(
  node: BuiltinLayoutNodeDescriptor,
  context: BuiltinLayoutContext,
): boolean {
  return node.isEligible(context);
}

export function defaultBuiltinLayoutContext(): BuiltinLayoutContext {
  return {
    hasWorkspace: true,
    inOffice: false,
    features: { coordinator: true, canvases: true, needsYouInbox: true },
  };
}
