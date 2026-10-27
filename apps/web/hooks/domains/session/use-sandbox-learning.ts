"use client";

import { useRef, useState } from "react";
import { useIsMutating, useMutation, useQueryClient } from "@tanstack/react-query";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import {
  proposeSandboxLearning,
  publishSandboxLearning,
  type SandboxLearningRequest,
  type SandboxLearningProposal,
} from "@/lib/api/domains/sandbox-learning-api";
import type { SandboxRevision } from "@/lib/types/sandbox";
import { getBackendConfig } from "@/lib/config";
import { sandboxApprovalContext } from "@/lib/sandbox-approval-context";
import { isSandboxReportBound } from "@/lib/sandbox-report";
import { submitSandboxOnce } from "@/lib/sandbox-submission";

const STALE_LEARNING = Symbol();
type Request = Omit<SandboxLearningRequest, "report_digest">;
type Action =
  | { key: string; request: Request }
  | { key: string; proposal: SandboxLearningProposal };
type Outcome = { proposal: SandboxLearningProposal } | { revision: SandboxRevision };

export function useSandboxLearning(sessionId: string, report: SandboxReport) {
  const store = useAppStoreApi();
  const client = useQueryClient();
  const backend = getBackendConfig().apiBaseUrl;
  const context = useAppStore(
    useShallow((state) => sandboxApprovalContext(state, sessionId, backend)),
  );
  const key = JSON.stringify([context.key, report.digest]);
  const latestKey = useRef(key);
  latestKey.current = key;
  const [review, setReview] = useState<{ key: string; proposal: SandboxLearningProposal } | null>(
    null,
  );
  const [published, setPublished] = useState<{ key: string; revision: SandboxRevision } | null>(
    null,
  );
  const proposal = review?.key === key ? review.proposal : null;
  const revision = published?.key === key ? published.revision : null;
  const readable = canReadLearning(context, report);
  const mutationKey = ["sandbox-learning", sessionId, key];
  const mutation = useMutation({
    mutationKey,
    retry: false,
    mutationFn: (action: Action) =>
      submitSandboxOnce<Outcome>(client, JSON.stringify(["learning", action]), async () => {
        const current = sandboxApprovalContext(store.getState(), sessionId, backend);
        if (
          !readable ||
          !current.enabled ||
          !current.operator ||
          current.key !== context.key ||
          latestKey.current !== action.key
        )
          throw STALE_LEARNING;
        if ("request" in action) {
          const value = await proposeSandboxLearning(sessionId, report.attempt_id, {
            ...action.request,
            report_digest: report.digest,
          });
          if (!isProposalBound(value, report)) throw STALE_LEARNING;
          return { proposal: value };
        }
        if (!proposal || proposal.digest !== action.proposal.digest || revision)
          throw STALE_LEARNING;
        return { revision: await publishSandboxLearning(action.proposal) };
      }),
  });
  const isBusy = useIsMutating({ mutationKey, exact: true }) > 0;
  async function submit(action: Action) {
    try {
      const value = await mutation.mutateAsync(action);
      if (latestKey.current !== action.key) return false;
      if ("proposal" in value) {
        setReview({ key, proposal: value.proposal });
        setPublished(null);
      } else setPublished({ key, revision: value.revision });
      return true;
    } catch {
      if ("proposal" in action && latestKey.current === action.key) setReview(null);
      return false;
    }
  }
  return {
    proposal,
    revision,
    snapshot: context.snapshot,
    isBusy,
    canPropose: readable && !isBusy && !revision,
    canPublish: readable && !isBusy && Boolean(proposal) && !revision,
    failed: mutation.isError && mutation.variables?.key === key,
    propose: (request: Request) => submit({ key, request }),
    publish: () => (proposal ? submit({ key, proposal }) : Promise.resolve(false)),
  };
}

function isProposalBound(proposal: SandboxLearningProposal, report: SandboxReport) {
  return (
    proposal.schema_version === 1 &&
    proposal.attempt_id === report.attempt_id &&
    proposal.session_id === report.session_id &&
    proposal.report_digest === report.digest &&
    proposal.report_version === report.version &&
    proposal.base_revision_id === report.revision_id &&
    proposal.base_digest === report.policy_digest &&
    proposal.ceiling_digest === report.ceiling_digest &&
    /^[a-f0-9]{64}$/.test(proposal.digest)
  );
}

function canReadLearning(
  context: ReturnType<typeof sandboxApprovalContext>,
  report: SandboxReport,
) {
  return (
    context.enabled &&
    context.operator &&
    isSandboxReportBound(report, context.snapshot) &&
    report.data.status === "available" &&
    report.data.integrity !== "unverified"
  );
}
