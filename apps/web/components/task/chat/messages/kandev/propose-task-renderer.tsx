"use client";

import { IconBulb } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { ProposalCard, type ProposalCardForm } from "@/app/coordinator/proposal-card/proposal-card";
import { isCreateTaskProposal, type StoredProposal } from "@/lib/api/domains/coordinator-api";
import { useProposalById } from "@/hooks/domains/coordinator/use-proposals";
import { useProposalWorkflowNames } from "@/hooks/domains/coordinator/use-proposal-workflow-names";
import { linkToCoordinatorNeedsYouForm } from "@/lib/coordinator/links";
import { effectiveProposalSpec } from "@/lib/coordinator/proposal-text";
import { useRouter } from "@/lib/routing/client-router";
import {
  useCoordinatorProposalContext,
  type CoordinatorProposalContextValue,
} from "./coordinator-proposal-context";
import { pickString } from "./parse";
import { KandevRow, type KandevStatus } from "./shared";
import type { KandevRenderer } from "./types";

// The plain "Kandev: Propose Task" row, with no expandable content: what a
// tool call whose result is an error, or whose text has no string
// `proposal_id`, renders (proposal-cards.md#cards).
function PlainProposeTaskRow({ status }: { status: KandevStatus }) {
  const { t } = useTranslation();
  return (
    <div data-testid="propose-task-renderer">
      <KandevRow
        Icon={IconBulb}
        title={t("task:kandevProposeTask")}
        status={status}
        hasExpandableContent={false}
      />
    </div>
  );
}

function proposalWorkflowId(proposal: StoredProposal): string | undefined {
  if (isCreateTaskProposal(proposal)) return effectiveProposalSpec(proposal).workflow_id;
  if (proposal.kind === "move") return (proposal.final_spec ?? proposal.spec).workflow_id;
  return undefined;
}

function ConnectedProposalCard({
  proposalId,
  ctx,
}: {
  proposalId: string;
  ctx: CoordinatorProposalContextValue;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const { proposal, notFound } = useProposalById(ctx.workspaceId, ctx.coordinatorId, proposalId);
  const workflowId = proposal ? (proposalWorkflowId(proposal) ?? null) : null;
  const { workflowNameById, stepNameByWorkflowStep } = useProposalWorkflowNames(workflowId);

  if (notFound) {
    return <p className="text-xs text-muted-foreground">{t("coordinator:toastNotFound")}</p>;
  }
  if (!proposal) {
    return <p className="text-xs text-muted-foreground">{t("coordinator:chatProposalLoading")}</p>;
  }

  function handleNavigateToForm(form: ProposalCardForm) {
    router.push(
      linkToCoordinatorNeedsYouForm(ctx.workspaceId, ctx.coordinatorId, proposalId, form),
    );
    ctx.closePopover();
  }

  return (
    <ProposalCard
      variant="compact"
      proposal={proposal}
      // The copilot popover this card renders inside is gated on
      // `workspace.manage` end-to-end (`useCoordinatorCopilot`), so a
      // reader never mounts this renderer.
      canManage
      workspaceId={ctx.workspaceId}
      coordinatorId={ctx.coordinatorId}
      workflowNameById={workflowNameById}
      stepNameByWorkflowStep={stepNameByWorkflowStep}
      onNavigateToForm={handleNavigateToForm}
    />
  );
}

// ProposeTaskRenderer attaches the `ProposalCard` to the `propose_task_kandev`
// tool call by the `proposal_id` in its JSON result text
// (proposal-cards.md#cards). It reads the card's own row independently of
// the Needs-you pending list, via `useProposalById`, so a reload can still
// show a settled proposal with no other proposal ever having been listed.
export const ProposeTaskRenderer: KandevRenderer = ({ result, status }) => {
  // Both hooks run unconditionally on every call, before the branch below,
  // because this renderer is invoked as a plain function inside
  // `KandevToolMessage`'s render rather than as its own JSX element — a
  // conditional hook call here would corrupt that caller's hook order.
  const { t } = useTranslation();
  const ctx = useCoordinatorProposalContext();
  const proposalId = pickString(result, "proposal_id");

  if (status === "error" || !proposalId || !ctx) {
    return <PlainProposeTaskRow status={status} />;
  }

  // The card is attached to the tool-call row, not nested inside its
  // collapsible body: unlike every other Kandev tool, this one exists so the
  // manager can act on it (Approve/Edit/Reject), so it must not be hidden
  // behind an expand toggle the way KandevBody content normally is.
  return (
    <div data-testid="propose-task-renderer">
      <KandevRow
        Icon={IconBulb}
        title={t("task:kandevProposeTask")}
        status={status}
        hasExpandableContent={false}
      />
      <div className="mt-2 ml-7">
        <ConnectedProposalCard proposalId={proposalId} ctx={ctx} />
      </div>
    </div>
  );
};
