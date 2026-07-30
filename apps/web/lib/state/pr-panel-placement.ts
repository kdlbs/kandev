import type { DockviewApi } from "dockview-react";
import type { PRPanelPlacement } from "@/lib/types/http";
import {
  CENTER_GROUP,
  RIGHT_BOTTOM_GROUP,
  SIDEBAR_GROUP,
  isCenterCandidateGroupId,
} from "./layout-manager";

type DockviewGroup = DockviewApi["groups"][number];

export type PRPanelOpenOptions = {
  activeSessionId?: string | null;
  placement?: PRPanelPlacement;
};

export type ResolvePRPanelTargetOptions = PRPanelOpenOptions & {
  centerGroupId: string;
  rightTopGroupId: string;
};

function resolveAgentGroupId(api: DockviewApi, options: ResolvePRPanelTargetOptions): string {
  const sessionGroupId = options.activeSessionId
    ? api.getPanel(`session:${options.activeSessionId}`)?.group?.id
    : undefined;
  if (sessionGroupId && isCenterCandidateGroupId(sessionGroupId)) return sessionGroupId;
  return isCenterCandidateGroupId(options.centerGroupId) ? options.centerGroupId : CENTER_GROUP;
}

function getAgentGroupIds(api: DockviewApi): Set<string> {
  const ids = new Set<string>();
  for (const panel of api.panels ?? []) {
    if (panel.id === "chat" || panel.id.startsWith("session:")) ids.add(panel.group.id);
  }
  return ids;
}

function isEligibleRightGroup(group: DockviewGroup, agentGroupIds: Set<string>): boolean {
  if (group.id === SIDEBAR_GROUP || agentGroupIds.has(group.id)) return false;
  return !(group.panels ?? []).some((panel) => panel.id === "sidebar");
}

function readGroupBounds(group: DockviewGroup): DOMRect | null {
  const element = group.element as HTMLElement | undefined;
  if (!element?.getBoundingClientRect) return null;
  const bounds = element.getBoundingClientRect();
  return bounds.width > 0 && bounds.height > 0 ? bounds : null;
}

function findVisuallyRightmostGroup(groups: DockviewGroup[]): DockviewGroup | undefined {
  const measured = groups.flatMap((group) => {
    const bounds = readGroupBounds(group);
    return bounds ? [{ group, bounds }] : [];
  });
  measured.sort(
    (a, b) =>
      b.bounds.right - a.bounds.right ||
      a.bounds.top - b.bounds.top ||
      a.group.id.localeCompare(b.group.id),
  );
  return measured[0]?.group;
}

function resolveRightGroupId(
  api: DockviewApi,
  options: ResolvePRPanelTargetOptions,
  agentGroupId: string,
): string | null {
  const agentGroupIds = getAgentGroupIds(api);
  agentGroupIds.add(agentGroupId);
  const groups = api.groups ?? [];
  const designated = groups.find((group) => group.id === options.rightTopGroupId);
  if (designated && isEligibleRightGroup(designated, agentGroupIds)) return designated.id;

  const eligible = groups.filter((group) => isEligibleRightGroup(group, agentGroupIds));
  const visuallyRightmost = findVisuallyRightmostGroup(eligible);
  if (visuallyRightmost) return visuallyRightmost.id;

  const knownRightBottom = eligible.find((group) => group.id === RIGHT_BOTTOM_GROUP);
  return knownRightBottom?.id ?? null;
}

export function resolvePRPanelTargetGroup(
  api: DockviewApi,
  options: ResolvePRPanelTargetOptions,
): string {
  const agentGroupId = resolveAgentGroupId(api, options);
  if (options.placement !== "right") return agentGroupId;
  return resolveRightGroupId(api, options, agentGroupId) ?? agentGroupId;
}
