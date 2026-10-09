"use client";

import { useEffect, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { selectWorkspaceById } from "@/lib/state/slices/workspace/selectors";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "@/lib/coordinator/links";
import { CoordinatorCopilot } from "./copilot/coordinator-copilot";
import { useResolvedCoordinator } from "./use-resolved-coordinator";
import {
  useCoordinatorAttention,
  type UseCoordinatorAttentionResult,
} from "./use-coordinator-attention";
import { PageShell } from "@/components/page-shell";
import {
  CoordinatorConfigureAction,
  CoordinatorTitleSlot,
  type CoordinatorHeaderView,
} from "./components/coordinator-header";
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

type CoordinatorScreenListProps = CoordinatorReadyContext & {
  workspaceId: string;
  view: CoordinatorHeaderView;
  tasksHardFailed: boolean;
  children: (ctx: CoordinatorReadyContext) => ReactNode;
};

/** The scrolling screen content: count strip, failure banner and the ready
 *  children, which the copilot panel sits beside. */
function CoordinatorScreenList({
  workspaceId,
  view,
  coordinator,
  coordinators,
  attention,
  canManage,
  tasksHardFailed,
  children,
}: CoordinatorScreenListProps) {
  return (
    <div className="h-full min-h-0 overflow-y-auto">
      {/* Full width under the topbar, not inside the content column: the
          derived-facts caption sits beside the counts, which only fits when
          the strip spans the window (mockup v2.1 `.strip`). */}
      {!tasksHardFailed && !attention.watchSetUnavailable && (
        <div className="bg-background sticky top-0 z-10 border-b px-4">
          <CountStrip
            classification={attention.classification}
            workspaceId={workspaceId}
            coordinatorId={coordinator.id}
            view={view}
          />
        </div>
      )}
      <div className="w-full max-w-3xl space-y-4 p-4">
        <InputFailureBanner inputs={attention.inputs} retry={attention.retryFailed} />
        {!tasksHardFailed && children({ coordinator, coordinators, attention, canManage })}
      </div>
    </div>
  );
}

/**
 * Shared shell for the Needs you and Queue screens: resolves the viewed
 * coordinator, renders the page chrome (the topbar crumb carrying the
 * coordinator and the screen, **Configure** for managers), the per-input
 * failure banner, and the loading/missing/error states, delegating the ready
 * content to `children`
 * (docs/specs/coordinator/system-design/needs-you.md#routes-and-sidebar,
 * #screens, #failure-and-recovery).
 *
 * The chrome is owned here rather than by each page client because the
 * coordinator it names is not known until this component has resolved it.
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
  const workspace = useAppStore(selectWorkspaceById(workspaceId));
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);

  const title = view === "queue" ? t("coordinator:queueTitle") : t("coordinator:needsYouTitle");
  // Until a coordinator resolves there is none to name, so the crumb is the
  // screen alone and the topbar carries no action.
  const bareShell = (inner: ReactNode) => (
    <PageShell title={title} topbarTestId="coordinator-topbar">
      {inner}
    </PageShell>
  );

  if (resolved.status === "loading") {
    return bareShell(
      <p className="text-muted-foreground p-4 text-sm" role="status" aria-live="polite">
        {t("common:loading")}
      </p>,
    );
  }
  if (resolved.status === "list-error") {
    return bareShell(<ListErrorState retry={resolved.retry} />);
  }
  if (resolved.status === "no-coordinator") {
    return bareShell(<NoCoordinatorState workspaceId={workspaceId} canManage={canManage} />);
  }
  if (resolved.status === "unknown-coordinator") {
    return bareShell(<UnknownCoordinatorState workspaceId={workspaceId} />);
  }
  if (resolved.status === "redirect") {
    return bareShell(
      <RedirectToCoordinator href={hrefForView(workspaceId, resolved.target.id, view)} />,
    );
  }

  // No workflow snapshot present and the read failed: lists and the count
  // strip are replaced by the banner (#failure-and-recovery).
  const tasksInput = attention.inputs.find((input) => input.kind === "tasks");
  const tasksHardFailed = attention.tasksNeverLoaded && Boolean(tasksInput?.error);

  const list = (
    <CoordinatorScreenList
      workspaceId={workspaceId}
      view={view}
      coordinator={resolved.coordinator}
      coordinators={resolved.coordinators}
      attention={attention}
      canManage={canManage}
      tasksHardFailed={tasksHardFailed}
    >
      {children}
    </CoordinatorScreenList>
  );

  return (
    <PageShell
      title={title}
      titleSlot={
        <CoordinatorTitleSlot
          coordinator={resolved.coordinator}
          coordinators={resolved.coordinators}
          workspaceId={workspaceId}
          view={view}
          title={title}
        />
      }
      actions={
        canManage ? (
          <CoordinatorConfigureAction
            coordinator={resolved.coordinator}
            workspaceId={workspaceId}
          />
        ) : undefined
      }
      topbarTestId="coordinator-topbar"
      scroll="none"
    >
      <div className="min-h-0 flex-1">
        {/* Keyed on the viewed coordinator: a coordinator switch fully
          remounts the controller, so every hook, ref, and draft resets to
          its initial value by construction instead of relying on each
          hook to detect and unwind a `coordinatorId` change itself
          (docs/specs/coordinator/system-design/copilot-popover.md). The
          panel wraps the list so it narrows beside the inline panel. */}
        <CoordinatorCopilot
          key={resolved.coordinator.id}
          workspaceId={workspaceId}
          coordinatorId={resolved.coordinator.id}
          coordinatorName={resolved.coordinator.name}
          canManage={canManage}
        >
          {list}
        </CoordinatorCopilot>
      </div>
    </PageShell>
  );
}
