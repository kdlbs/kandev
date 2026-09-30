"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import Link from "@/components/routing/app-link";
import TaskLink from "@/components/routing/task-link";
import { ContextDiff } from "@/components/coordinators/context-diff";
import { ApiError } from "@/lib/api/client";
import type { ImprovementProposal } from "@/lib/api/domains/coordinator-api";
import type {
  ImprovementEvidence,
  PendingChangeStatus,
} from "@/lib/api/domains/coordinator-changes-api";
import { getRun, type RunRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";
import { linkToCoordinatorAutonomySettings } from "@/lib/coordinator/links";

type TFn = ReturnType<typeof useTranslation>["t"];

const APPROVED_LINE_KEY: Record<PendingChangeStatus, string> = {
  pending: "coordinator:improvementApprovedPending",
  applied: "coordinator:improvementApprovedApplied",
  discarded: "coordinator:improvementApprovedDiscarded",
};

/** The approved line of an improvement card, chosen by the state of its change. */
export function improvementApprovedLine(proposal: ImprovementProposal, t: TFn): string {
  return t(APPROVED_LINE_KEY[proposal.change_status ?? "pending"]);
}

/** The failed line: one sentence for every error code, never the error text. */
export function improvementFailedLine(t: TFn): string {
  return t("coordinator:improvementFailed");
}

type RunState =
  | { status: "loading" }
  | { status: "loaded"; run: RunRead }
  | { status: "expired" }
  | { status: "unavailable" };

function useRunEvidence(workspaceId: string, coordinatorId: string, runId: string): RunState {
  const [state, setState] = useState<RunState>({ status: "loading" });
  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    getRun(workspaceId, coordinatorId, runId)
      .then((run) => !cancelled && setState({ status: "loaded", run }))
      .catch((error: unknown) => {
        if (cancelled) return;
        const expired = error instanceof ApiError && error.status === 404;
        setState({ status: expired ? "expired" : "unavailable" });
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, coordinatorId, runId]);
  return state;
}

function RunLine({ run }: { run: RunRead }) {
  const { t, i18n } = useTranslation();
  const started = new Date(run.started_at).toLocaleString(i18n.language);
  const outcome = run.outcome ?? t("coordinator:improvementRunInProgress");
  const cost = run.cost_subcents === null ? null : formatSubcentsUsd(run.cost_subcents);
  return (
    <span data-testid="improvement-run-line">
      {[started, outcome, cost].filter((part) => part !== null).join(" · ")}
    </span>
  );
}

function RunEvidenceRow({
  workspaceId,
  coordinatorId,
  runId,
}: {
  workspaceId: string;
  coordinatorId: string;
  runId: string;
}) {
  const { t } = useTranslation();
  const state = useRunEvidence(workspaceId, coordinatorId, runId);
  let body: React.ReactNode = t("coordinator:improvementRunLoading");
  if (state.status === "loaded") body = <RunLine run={state.run} />;
  if (state.status === "expired") body = t("coordinator:improvementRunExpired");
  if (state.status === "unavailable") body = t("coordinator:improvementRunUnavailable");
  return (
    <li className="text-xs text-muted-foreground" data-testid="improvement-evidence-run">
      {body}
    </li>
  );
}

type TaskState = { status: "loading" } | { status: "loaded"; label: string } | { status: "gone" };

function useTaskEvidence(taskId: string, openTasksById?: Map<string, AttentionTask>): TaskState {
  const mapped = openTasksById?.get(taskId);
  const [state, setState] = useState<TaskState>({ status: "loading" });
  useEffect(() => {
    if (mapped) return;
    let cancelled = false;
    fetchTask(taskId)
      .then((task) => {
        if (cancelled) return;
        const label = [task.identifier, task.title].filter(Boolean).join(" ");
        setState(label ? { status: "loaded", label } : { status: "gone" });
      })
      .catch(() => !cancelled && setState({ status: "gone" }));
    return () => {
      cancelled = true;
    };
  }, [mapped, taskId]);
  if (mapped) {
    return {
      status: "loaded",
      label: [mapped.identifier, mapped.title].filter(Boolean).join(" ") || mapped.id,
    };
  }
  return state;
}

function TaskEvidenceRow({
  taskId,
  openTasksById,
}: {
  taskId: string;
  openTasksById?: Map<string, AttentionTask>;
}) {
  const { t } = useTranslation();
  const state = useTaskEvidence(taskId, openTasksById);
  let body: React.ReactNode = t("coordinator:improvementRunLoading");
  if (state.status === "loaded") {
    body = (
      <TaskLink taskId={taskId} className="cursor-pointer underline">
        {state.label}
      </TaskLink>
    );
  }
  if (state.status === "gone") body = t("coordinator:improvementTaskGone");
  return (
    <li className="text-xs text-muted-foreground" data-testid="improvement-evidence-task">
      {body}
    </li>
  );
}

function EvidenceList({
  evidence,
  workspaceId,
  coordinatorId,
  openTasksById,
}: {
  evidence: ImprovementEvidence[];
  workspaceId: string;
  coordinatorId: string;
  openTasksById?: Map<string, AttentionTask>;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium">{t("coordinator:improvementRunsHeading")}</p>
      <ul className="space-y-0.5">
        {evidence.map((entry, index) =>
          entry.run_id ? (
            <RunEvidenceRow
              key={`${index}-${entry.run_id}`}
              workspaceId={workspaceId}
              coordinatorId={coordinatorId}
              runId={entry.run_id}
            />
          ) : (
            <TaskEvidenceRow
              key={`${index}-${entry.task_id}`}
              taskId={entry.task_id ?? ""}
              openTasksById={openTasksById}
            />
          ),
        )}
      </ul>
    </div>
  );
}

type ImprovementBodyProps = {
  proposal: ImprovementProposal;
  workspaceId: string;
  coordinatorId: string;
  openTasksById?: Map<string, AttentionTask>;
  shown: boolean;
  onShow: () => void;
};

/** Title, rationale, evidence and the reveal-able diff of an improvement card. */
export function ImprovementBody({
  proposal,
  workspaceId,
  coordinatorId,
  openTasksById,
  shown,
  onShow,
}: ImprovementBodyProps) {
  const { t } = useTranslation();
  const spec = proposal.spec;
  return (
    <div className="space-y-2" data-testid="improvement-body">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium uppercase text-muted-foreground">
          {t("coordinator:improvementHeader")}
        </span>
        <span className="rounded-full border px-2 py-0.5 text-xs">
          {t("coordinator:improvementPill")}
        </span>
      </div>
      <p className="text-sm font-medium">{spec.title}</p>
      <CardDescription className="whitespace-pre-wrap">{spec.rationale}</CardDescription>
      <EvidenceList
        evidence={spec.evidence}
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        openTasksById={openTasksById}
      />
      <ImprovementOpenSettings
        proposal={proposal}
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
      />
      {shown ? (
        <ContextDiff before={spec.context_before} after={spec.context_after} />
      ) : (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={onShow}
          data-testid="improvement-show-change"
          className="min-h-11 sm:min-h-0"
        >
          {t("coordinator:improvementShowChange")}
        </Button>
      )}
    </div>
  );
}

/** The gate an improvement card puts on Approve and Retry; null on any other card. */
export function improvementGateOf(
  proposal: ImprovementProposal | null,
  diffShown: boolean,
): { diffShown: boolean } | null {
  return proposal ? { diffShown } : null;
}

/** The Open settings link of an approved improvement whose change is still pending. */
function ImprovementOpenSettings({
  proposal,
  workspaceId,
  coordinatorId,
}: {
  proposal: ImprovementProposal;
  workspaceId: string;
  coordinatorId: string;
}) {
  const { t } = useTranslation();
  if (proposal.status !== "approved") return null;
  const status = proposal.change_status ?? "pending";
  if (status !== "pending") return null;
  return (
    <Link
      href={linkToCoordinatorAutonomySettings(workspaceId, coordinatorId)}
      className="text-sm text-primary hover:underline"
      data-testid="improvement-open-settings"
    >
      {t("coordinator:improvementOpenSettings")}
    </Link>
  );
}
