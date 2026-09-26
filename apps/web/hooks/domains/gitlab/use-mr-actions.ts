"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";

/**
 * A stable identifier for the action in flight, never display copy.
 *
 * This used to be the human label itself — `run("Approve", …)` — which made one
 * string do two jobs: it was stored in `pendingAction` state AND interpolated
 * into the failure toast as `` `${label} failed` ``. That is the dual-use shape
 * apps/web/AGENTS.md warns about: translating it in place would have changed a
 * state value, and leaving it English would have left the toast title
 * un-migrated. Splitting them lets the id stay fixed while the copy becomes a
 * catalog key, and it removes a sentence built by concatenation, which no
 * language that inflects the verb could have translated anyway.
 */
export type MRActionKind =
  | "approve"
  | "unapprove"
  | "reviewers"
  | "assignees"
  | "merge"
  | "unlink"
  | "reply"
  | "resolve"
  | "labels";

/**
 * Catalog keys per action. Written as plain `ns:key` literals so
 * `check-i18n-keys.mjs` counts them as referenced — a key built by template
 * would be invisible to it and every entry would report as an orphan.
 */
const ACTION_COPY: Record<MRActionKind, { name: string; success: string }> = {
  approve: { name: "gitlab:actionApprove", success: "gitlab:mergeRequestApproved" },
  unapprove: { name: "gitlab:actionUnapprove", success: "gitlab:approvalRemoved" },
  reviewers: { name: "gitlab:actionReviewers", success: "gitlab:reviewersUpdated" },
  assignees: { name: "gitlab:actionAssignees", success: "gitlab:assigneesUpdated" },
  merge: { name: "gitlab:actionMerge", success: "gitlab:mergeRequestMerged" },
  unlink: { name: "gitlab:actionUnlink", success: "gitlab:mergeRequestUnlinked" },
  reply: { name: "gitlab:actionReply", success: "gitlab:replyAdded" },
  resolve: { name: "gitlab:actionResolve", success: "gitlab:discussionResolved" },
  labels: { name: "gitlab:actionLabels", success: "gitlab:labelsUpdated" },
};

export function useMRActions(onRefresh: () => void) {
  const [pendingAction, setPendingAction] = useState<MRActionKind | null>(null);
  const { toast } = useToast();
  const { t } = useTranslation();

  const run = useCallback(
    async (kind: MRActionKind, action: () => Promise<unknown>) => {
      const copy = ACTION_COPY[kind];
      setPendingAction(kind);
      try {
        await action();
        toast({ description: t(copy.success), variant: "success" });
        onRefresh();
        return true;
      } catch (error) {
        toast({
          title: t("gitlab:actionFailed", { action: t(copy.name) }),
          // A GitLab API error is a diagnostic and stays English; the fallback
          // for a non-Error rejection is copy.
          description: error instanceof Error ? error.message : t("gitlab:gitlabRejectedAction"),
          variant: "error",
        });
        return false;
      } finally {
        setPendingAction(null);
      }
    },
    [onRefresh, t, toast],
  );

  return { pendingAction, run };
}
