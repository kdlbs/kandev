import { useTranslation } from "react-i18next";
import type { MergePipeline, MergeTri } from "@/lib/api/domains/coordinator-merge-api";
import {
  approvalsText,
  conflictText,
  countText,
  draftText,
  mergeableText,
  pipelineText,
} from "@/lib/coordinator/merge-text";

export type MergeConditionValues = {
  pipeline: MergePipeline;
  mergeable: MergeTri;
  conflict: MergeTri;
  approvals: MergeTri;
  draft: MergeTri;
  /** null when the count could not be read. */
  threads: number | null;
};

function Row({ label, value, state }: { label: string; value: string; state: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1 sm:block">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm" data-state={state}>
        {value}
      </dd>
    </div>
  );
}

function pipelineState(p: MergePipeline): string {
  if (p === "green") return "met";
  return p === "unavailable" ? "unavailable" : "unmet";
}

function threadsState(count: number | null): string {
  if (count === null) return "unavailable";
  return count === 0 ? "met" : "unmet";
}

/**
 * The provider conditions of a change, each labelled; an unread value says unavailable
 * and is never presented as passing.
 */
export function MergeConditions({
  values,
  testId = "merge-conditions",
}: {
  values: MergeConditionValues;
  testId?: string;
}) {
  const { t } = useTranslation();
  return (
    <dl className="grid grid-cols-1 gap-x-4 sm:grid-cols-3" data-testid={testId}>
      <Row
        label={t("coordinator:mergeCondPipeline")}
        value={pipelineText(values.pipeline, t)}
        state={pipelineState(values.pipeline)}
      />
      <Row
        label={t("coordinator:mergeCondMergeable")}
        value={mergeableText(values.mergeable, t)}
        state={values.mergeable}
      />
      <Row
        label={t("coordinator:mergeCondConflicts")}
        value={conflictText(values.conflict, t)}
        state={values.conflict}
      />
      <Row
        label={t("coordinator:mergeCondApprovals")}
        value={approvalsText(values.approvals, t)}
        state={values.approvals}
      />
      <Row
        label={t("coordinator:mergeCondThreads")}
        value={countText("coordinator:unresolvedThreads", values.threads, t)}
        state={threadsState(values.threads)}
      />
      <Row
        label={t("coordinator:mergeCondDraft")}
        value={draftText(values.draft, t)}
        state={values.draft}
      />
    </dl>
  );
}
