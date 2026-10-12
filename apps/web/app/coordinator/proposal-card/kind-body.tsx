"use client";

import { Trans, useTranslation } from "react-i18next";
import { CardDescription } from "@kandev/ui/card";
import TaskLink from "@/components/routing/task-link";
import type { KindProposal, MessageSpec, MoveSpec } from "@/lib/api/domains/coordinator-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import { useTargetTask } from "./use-target-task";

type KindBodyProps = {
  proposal: KindProposal;
  openTasksById: Map<string, AttentionTask> | undefined;
  stepNameByWorkflowStep: Map<string, string>;
};

function stepName(spec: MoveSpec, stepId: string, names: Map<string, string>): string {
  return names.get(`${spec.workflow_id}:${stepId}`) ?? stepId;
}

function cardComponent(taskId: string, known: boolean) {
  if (!known) return <span />;
  return <TaskLink taskId={taskId} className="cursor-pointer underline" />;
}

function titleKey(proposal: KindProposal, hasFrom: boolean): string {
  if (proposal.kind === "resume") return "coordinator:kindTitleResume";
  if (proposal.kind === "message") return "coordinator:kindTitleMessage";
  return hasFrom ? "coordinator:kindTitleMove" : "coordinator:kindTitleMoveNoFrom";
}

function KindTitle({
  proposal,
  stepNameByWorkflowStep,
  label,
  known,
}: KindBodyProps & { label: string; known: boolean }) {
  const spec = proposal.final_spec ?? proposal.spec;
  const move = proposal.kind === "move" ? (spec as MoveSpec) : null;
  const to = move ? stepName(move, move.to_step_id, stepNameByWorkflowStep) : "";
  const from =
    move && move.from_step_id ? stepName(move, move.from_step_id, stepNameByWorkflowStep) : "";
  return (
    <p className="text-sm font-medium">
      <Trans
        i18nKey={titleKey(proposal, from !== "")}
        values={{ card: label, from, to }}
        components={{ card: cardComponent(spec.task_id, known) }}
      />
    </p>
  );
}

/** Title, message text, rationale and starts-agent line of a resume, message or move card. */
export function KindBody(props: KindBodyProps) {
  const { t } = useTranslation();
  const { proposal } = props;
  const spec = proposal.final_spec ?? proposal.spec;
  const target = useTargetTask(spec.task_id, props.openTasksById);
  return (
    <>
      <KindTitle {...props} label={target.label} known={target.known} />
      {proposal.kind === "message" && (
        <blockquote className="text-sm border-l-2 pl-2 whitespace-pre-wrap">
          {(spec as MessageSpec).text}
        </blockquote>
      )}
      <CardDescription>{spec.rationale}</CardDescription>
      {proposal.starts_agent === true && (
        <CardDescription>{t("coordinator:startsAgentLine")}</CardDescription>
      )}
    </>
  );
}
