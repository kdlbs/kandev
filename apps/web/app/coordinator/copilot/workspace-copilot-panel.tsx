"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useStore } from "zustand";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import AppLink from "@/components/routing/app-link";
import { linkToCoordinatorNeedsYou } from "@/lib/coordinator/links";
import type { WorkspaceCopilotStore } from "@/hooks/domains/coordinator/workspace-copilot-store";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";
import { CoordinatorCopilotPanelContent } from "./coordinator-copilot-panel-content";
import { CoordinatorSwitcher } from "./coordinator-switcher";
import { WorkspacePageChipRow } from "./workspace-page-chip";
import { useCoordinatorCopilot } from "./use-coordinator-copilot";
import { useCoordinatorWatches } from "./use-coordinator-watches";
import type { PageContext } from "./use-page-context";

export type WorkspaceCopilotPanelProps = {
  store: WorkspaceCopilotStore;
  workspaceId: string;
  coordinator: Coordinator;
  coordinators: readonly Coordinator[];
  context: PageContext | null;
  routeKey: string;
  onClose: () => void;
  onSwitch: (coordinatorId: string) => void;
  onGone: (coordinatorId: string) => void;
};

function notWatchedBy(
  context: PageContext | null,
  watches: ReturnType<typeof useCoordinatorWatches>,
): boolean {
  if (!context || !watches.loaded || watches.watches?.scope !== "selected") return false;
  return context.workflowId !== null && !watches.watches.workflow_ids.includes(context.workflowId);
}

/** The host's open panel: the phase-1 controller and content on the host store,
 *  with the page chip derived from the route instead of set by Ask about this. */
export function WorkspaceCopilotPanel({
  store,
  workspaceId,
  coordinator,
  coordinators,
  context,
  routeKey,
  onClose,
  onSwitch,
  onGone,
}: WorkspaceCopilotPanelProps) {
  const { t } = useTranslation();
  const copilot = useCoordinatorCopilot(workspaceId, coordinator.id, true, store);
  const dismissedFor = useStore(store, (s) => s.chipDismissedFor);
  const watches = useCoordinatorWatches({
    workspaceId,
    coordinatorId: coordinator.id,
    routeKey,
    enabled: true,
    onGone,
  });

  const gone = copilot.openSequence.state.kind === "gone" || copilot.launcher.gone;
  useEffect(() => {
    if (gone) onGone(coordinator.id);
  }, [gone, coordinator.id, onGone]);

  const chip: CopilotChip | null =
    context && dismissedFor !== routeKey
      ? { id: context.ref.id, label: context.label, ref: context.ref }
      : null;
  const chipRow = chip ? (
    <WorkspacePageChipRow
      chip={chip}
      notWatched={notWatchedBy(context, watches)}
      onRemove={() => store.getState().dismissChip(routeKey)}
    />
  ) : null;

  const headerActions = (
    <>
      <CoordinatorSwitcher coordinators={coordinators} value={coordinator.id} onChange={onSwitch} />
      <AppLink
        href={linkToCoordinatorNeedsYou(workspaceId, coordinator.id)}
        className="cursor-pointer text-xs underline-offset-2 hover:underline [@media(pointer:coarse)]:flex [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:items-center"
        data-testid="workspace-copilot-open-page"
        onNavigated={onClose}
      >
        {t("coordinator:copilotOpenCoordinatorPage")}
      </AppLink>
    </>
  );

  return (
    <CoordinatorCopilotPanelContent
      copilot={copilot}
      workspaceId={workspaceId}
      coordinatorId={coordinator.id}
      coordinatorName={coordinator.name}
      onClose={onClose}
      onClosePopover={onClose}
      headerActions={headerActions}
      chip={chip}
      chipRow={chipRow}
    />
  );
}
