"use client";

import { useEffect, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { selectActiveWorkspace } from "@/lib/state/slices/workspace/selectors";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "@/lib/coordinator/links";
import { useResolvedCoordinator } from "./use-resolved-coordinator";
import {
  useCoordinatorAttention,
  type UseCoordinatorAttentionResult,
} from "./use-coordinator-attention";
import { CoordinatorHeader, type CoordinatorHeaderView } from "./components/coordinator-header";
import { CountStrip } from "./components/count-strip";
import { InputFailureBanner } from "./components/input-failure-banner";
import { ListErrorState } from "./components/list-error-state";
import { NoCoordinatorState } from "./components/no-coordinator-state";
import { UnknownCoordinatorState } from "./components/unknown-coordinator-state";

export type CoordinatorReadyContext = {
  coordinator: Coordinator;
  coordinators: Coordinator[];
  attention: UseCoordinatorAttentionResult;
  canManage: boolean;
};

export type CoordinatorRouteContentProps = {
  workspaceId: string;
  coordinatorId: string | null;
  view: CoordinatorHeaderView;
  children: (ctx: CoordinatorReadyContext) => ReactNode;
};

function hrefForView(
  workspaceId: string,
  coordinatorId: string,
  view: CoordinatorHeaderView,
): string {
  return view === "queue"
    ? linkToCoordinatorQueue(workspaceId, coordinatorId)
    : linkToCoordinatorNeedsYou(workspaceId, coordinatorId);
}

function RedirectToCoordinator({ href }: { href: string }) {
  const router = useRouter();
  useEffect(() => {
    router.replace(href);
  }, [router, href]);
  return null;
}

/**
 * Shared shell for the Needs you and Queue screens: resolves the viewed
 * coordinator, renders the header, the per-input failure banner, and the
 * loading/missing/error states, delegating the ready content to `children`
 * (docs/specs/coordinator/system-design/needs-you.md#routes-and-sidebar,
 * #screens, #failure-and-recovery).
 */
export function CoordinatorRouteContent({
  workspaceId,
  coordinatorId,
  view,
  children,
}: CoordinatorRouteContentProps) {
  const { t } = useTranslation();
  const resolved = useResolvedCoordinator(workspaceId, coordinatorId);
  const readyCoordinatorId = resolved.status === "ready" ? resolved.coordinator.id : null;
  const attention = useCoordinatorAttention(workspaceId, readyCoordinatorId);
  useWorkspacePRs(workspaceId);
  const workspace = useAppStore(selectActiveWorkspace);
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);

  if (resolved.status === "loading") {
    return (
      <p className="text-muted-foreground p-4 text-sm" role="status" aria-live="polite">
        {t("common:loading")}
      </p>
    );
  }
  if (resolved.status === "list-error") {
    return <ListErrorState retry={resolved.retry} />;
  }
  if (resolved.status === "no-coordinator") {
    return <NoCoordinatorState workspaceId={workspaceId} canManage={canManage} />;
  }
  if (resolved.status === "unknown-coordinator") {
    return <UnknownCoordinatorState workspaceId={workspaceId} />;
  }
  if (resolved.status === "redirect") {
    return <RedirectToCoordinator href={hrefForView(workspaceId, resolved.target.id, view)} />;
  }

  // No workflow snapshot present and the read failed: lists and the count
  // strip are replaced by the banner (#failure-and-recovery).
  const tasksInput = attention.inputs.find((input) => input.kind === "tasks");
  const tasksHardFailed = attention.tasksNeverLoaded && Boolean(tasksInput?.error);

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4 p-4">
      <CoordinatorHeader
        coordinator={resolved.coordinator}
        coordinators={resolved.coordinators}
        workspaceId={workspaceId}
        view={view}
        canManage={canManage}
      />
      <InputFailureBanner inputs={attention.inputs} retry={attention.retryFailed} />
      {!tasksHardFailed && (
        <>
          <div className="bg-background sticky top-0 z-10">
            <CountStrip
              classification={attention.classification}
              workspaceId={workspaceId}
              coordinatorId={resolved.coordinator.id}
            />
          </div>
          {children({
            coordinator: resolved.coordinator,
            coordinators: resolved.coordinators,
            attention,
            canManage,
          })}
        </>
      )}
    </div>
  );
}
