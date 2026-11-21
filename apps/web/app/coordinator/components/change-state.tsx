"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import type { ChangeView } from "@/lib/api/domains/coordinator-merge-api";
import type { TaskChangesStatus } from "@/hooks/domains/coordinator/use-task-changes";
import { countText, shortSha } from "@/lib/coordinator/merge-text";
import { formatDateTime } from "@/lib/i18n/formats";
import { MergeConditions } from "./merge-conditions";

/** The zero time the backend sends for a change that was never read. */
function wasRead(change: ChangeView): boolean {
  const at = new Date(change.observed_at);
  return !Number.isNaN(at.getTime()) && at.getFullYear() > 1;
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1 sm:block">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-all">{value}</dd>
    </div>
  );
}

function ChangeCard({ change, onRetry }: { change: ChangeView; onRetry: () => void }) {
  const { t } = useTranslation();
  const read = wasRead(change);
  const unavailable = t("coordinator:mergeValUnavailable");
  return (
    <li
      className="space-y-2 py-3"
      data-testid={`queue-change-${change.number}`}
      data-unavailable={change.unavailable}
    >
      <p className="text-sm font-medium break-words">
        {t("coordinator:mergeChangeHeading", {
          provider: change.provider,
          repository: change.repository_id,
          number: change.number,
        })}
      </p>
      <dl className="grid grid-cols-1 gap-x-4 sm:grid-cols-3">
        <Field
          label={t("coordinator:mergeFieldTarget")}
          value={read && change.target_branch ? change.target_branch : unavailable}
        />
        <Field
          label={t("coordinator:mergeFieldHead")}
          value={read && change.head_sha ? shortSha(change.head_sha) : unavailable}
        />
        <Field
          label={t("coordinator:mergeFieldBehind")}
          value={countText("coordinator:mergeValBehind", read ? change.behind : null, t)}
        />
      </dl>
      <MergeConditions
        testId={`queue-change-conditions-${change.number}`}
        values={{
          pipeline: change.pipeline,
          mergeable: change.mergeable,
          conflict: change.conflict,
          approvals: change.approvals,
          draft: change.draft,
          threads: read ? change.unresolved_threads : null,
        }}
      />
      {change.unavailable && (
        <div className="space-y-2" data-testid={`queue-change-failed-${change.number}`}>
          <p role="status" className="text-sm">
            {read
              ? t("coordinator:mergeChangeStale", { time: formatDateTime(change.observed_at) })
              : t("coordinator:mergeChangeNeverRead")}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
            onClick={onRetry}
          >
            {t("coordinator:retry")}
          </Button>
        </div>
      )}
    </li>
  );
}

type Props = {
  changes: ChangeView[] | null;
  status: TaskChangesStatus;
  onRetry: () => void;
};

/** The linked changes of a card with every field; unread values say unavailable and offer Retry. */
export function ChangeState({ changes, status, onRetry }: Props) {
  const { t } = useTranslation();
  if (status === "error" && !changes) {
    return (
      <div className="space-y-2" data-testid="queue-changes-error">
        <p className="text-destructive text-sm">{t("coordinator:mergeChangesLoadFailed")}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-0"
          onClick={onRetry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (!changes) return <Skeleton className="h-4 w-48" data-testid="queue-changes-loading" />;
  if (changes.length === 0) {
    return <p className="text-muted-foreground text-sm">{t("coordinator:mergeChangesNone")}</p>;
  }
  return (
    <ul className="divide-border divide-y" data-testid="queue-changes">
      {changes.map((change) => (
        <ChangeCard key={change.key} change={change} onRetry={onRetry} />
      ))}
      {status === "error" && (
        <li className="py-2">
          <p role="alert" className="text-destructive text-sm">
            {t("coordinator:mergeChangesLoadFailed")}
          </p>
        </li>
      )}
    </ul>
  );
}
