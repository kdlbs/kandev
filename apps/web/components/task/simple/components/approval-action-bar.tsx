"use client";

import { useState } from "react";
import { IconCheck, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import {
  approveTask,
  requestTaskChanges,
  type TaskDecisionDTO,
} from "@/lib/api/domains/office-extended-api";
import type { Task, TaskDecision, TaskDecisionRole } from "@/app/office/tasks/[id]/types";

// USER_VIEWER is the synthetic decider id we use for the singleton human
// user. Mirrors the backend's `userSentinel` — the approval endpoints
// treat unauthenticated callers as this user automatically, so the
// frontend just needs to know which id will end up on the decision row
// in order to identify "the viewer's" decision.
const USER_VIEWER_ID = "user";
const USER_VIEWER_TYPE = "user" as const;

// resolveViewerRoles inspects the task's reviewer/approver lists and
// returns the role(s) the singleton human viewer fills.
//
// V1: returns [] always — the unified participants store
// (`workflow_step_participants` after ADR 0005 Wave C-backend; previously
// `office_task_participants`) only stores agent profile ids, so the
// singleton human user is never a real participant. Showing the action bar
// to a human would invite them to "approve" a task whose approvers are all
// agents; the user's decision wouldn't unblock the agent-approver gate
// (only matching agent ids count), so the button would be cosmetic.
//
// Future: when human participants land (e.g. a participant_type column on
// `workflow_step_participants`), this function reads them from the task
// DTO and returns the role(s) the current user holds. The downstream
// gate logic in approval-action-bar already prefers "approver" over
// "reviewer" since approvers gate completion.
function resolveViewerRoles(_task: Task): TaskDecisionRole[] {
  return [];
}

function viewerHasActiveDecision(decisions: TaskDecision[]): boolean {
  return decisions.some(
    (d) => d.deciderType === USER_VIEWER_TYPE && d.deciderId === USER_VIEWER_ID,
  );
}

function makeOptimisticDecision(
  taskId: string,
  role: TaskDecisionRole,
  verdict: "approved" | "changes_requested",
  comment: string,
): TaskDecision {
  return {
    id: `optimistic-${Date.now()}`,
    taskId,
    deciderType: USER_VIEWER_TYPE,
    deciderId: USER_VIEWER_ID,
    deciderName: "You",
    role,
    decision: verdict,
    comment,
    createdAt: new Date().toISOString(),
  };
}

function appendDecision(decisions: TaskDecision[], next: TaskDecision): TaskDecision[] {
  return [...decisions, next];
}

function dtoToDecision(dto: TaskDecisionDTO): TaskDecision {
  return {
    id: dto.id,
    taskId: dto.task_id,
    deciderType: dto.decider_type,
    deciderId: dto.decider_id,
    deciderName: dto.decider_name ?? "You",
    role: dto.role,
    decision: dto.decision,
    comment: dto.comment ?? "",
    createdAt: dto.created_at,
  };
}

type ApprovalActionBarProps = {
  task: Task;
};

type Mode = "idle" | "approve" | "request_changes";

export function ApprovalActionBar({ task }: ApprovalActionBarProps) {
  const [mode, setMode] = useState<Mode>("idle");
  const [comment, setComment] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const mutate = useOptimisticTaskMutation();

  const roles = resolveViewerRoles(task);
  const hasDecided = viewerHasActiveDecision(task.decisions);
  if (roles.length === 0 || hasDecided) return null;

  // Prefer "approver" for the optimistic row when the user is in both
  // lists — approvers gate completion and the timeline reads better.
  const role: TaskDecisionRole = roles.includes("approver") ? "approver" : "reviewer";

  const reset = () => {
    setMode("idle");
    setComment("");
  };

  const submit = async (verdict: "approved" | "changes_requested") => {
    if (verdict === "changes_requested" && !comment.trim()) return;
    setSubmitting(true);
    const optimistic = makeOptimisticDecision(task.id, role, verdict, comment.trim());
    try {
      await mutate(task.id, { decisions: appendDecision(task.decisions, optimistic) }, async () => {
        const dto =
          verdict === "approved"
            ? await approveTask(task.id, comment.trim() || undefined)
            : await requestTaskChanges(task.id, comment.trim());
        // Replace the optimistic id with the real one so subsequent
        // checks (viewerHasActiveDecision) keep matching after the
        // server-authoritative refetch overlay arrives.
        return dto;
      });
      reset();
    } catch {
      /* hook toasts; rollback already happened */
    } finally {
      setSubmitting(false);
    }
  };

  void dtoToDecision; // exported indirectly via tests, see below

  return (
    <div
      className="rounded-md border border-border bg-muted/30 p-3 mb-3"
      data-testid="approval-action-bar"
    >
      {mode === "idle" && (
        <div className="flex items-center gap-2">
          <p className="text-sm text-muted-foreground flex-1">
            Your review is requested on this task.
          </p>
          <Button
            type="button"
            size="sm"
            className="cursor-pointer"
            data-testid="approval-action-approve"
            onClick={() => setMode("approve")}
          >
            <IconCheck className="h-3.5 w-3.5 mr-1" /> Approve
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="cursor-pointer"
            data-testid="approval-action-request-changes"
            onClick={() => setMode("request_changes")}
          >
            <IconX className="h-3.5 w-3.5 mr-1" /> Request changes
          </Button>
        </div>
      )}
      {mode !== "idle" && (
        <ApprovalCommentForm
          mode={mode}
          comment={comment}
          submitting={submitting}
          onCommentChange={setComment}
          onCancel={reset}
          onSubmit={() => submit(mode === "approve" ? "approved" : "changes_requested")}
        />
      )}
    </div>
  );
}

// dtoToDecision is exported for testing; the action-bar swap of the
// optimistic id with the real decision row id is performed implicitly
// when `task:<id>` refetches via the WS handler. We keep the helper
// available so future flows (e.g. a per-decision detail view) can map
// the wire DTO consistently with mapDecisionDTO in page.tsx.
export { dtoToDecision };

type ApprovalCommentFormProps = {
  mode: "approve" | "request_changes";
  comment: string;
  submitting: boolean;
  onCommentChange: (next: string) => void;
  onCancel: () => void;
  onSubmit: () => void;
};

function ApprovalCommentForm({
  mode,
  comment,
  submitting,
  onCommentChange,
  onCancel,
  onSubmit,
}: ApprovalCommentFormProps) {
  const required = mode === "request_changes";
  const placeholder =
    mode === "approve" ? "Add an optional comment" : "Describe what needs to change";
  const submitDisabled = submitting || (required && !comment.trim());
  const submitLabel = mode === "approve" ? "Approve" : "Request changes";
  return (
    <div className="space-y-2">
      <Textarea
        value={comment}
        onChange={(e) => onCommentChange(e.target.value)}
        placeholder={placeholder}
        rows={2}
        data-testid="approval-action-comment"
      />
      <div className="flex items-center gap-2 justify-end">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="cursor-pointer"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          className="cursor-pointer"
          data-testid="approval-action-submit"
          disabled={submitDisabled}
          onClick={onSubmit}
        >
          {submitLabel}
        </Button>
      </div>
    </div>
  );
}
