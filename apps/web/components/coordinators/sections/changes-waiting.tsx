"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { ApiError } from "@/lib/api/client";
import { getCoordinator } from "@/lib/api/domains/coordinator-api";
import {
  applyPendingChange,
  discardPendingChange,
  type PendingChange,
} from "@/lib/api/domains/coordinator-changes-api";
import { usePendingChanges } from "@/hooks/domains/coordinator/use-pending-changes";
import { useSettingsSaveCoordinator } from "@/components/settings/settings-save-provider";
import { useSearchParams } from "@/lib/routing/client-router";
import { ContextDiff } from "@/components/coordinators/context-diff";

type Props = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  /** Receives the coordinator's stored context after an Apply succeeded. */
  onContextApplied?: (context: string) => void;
};

type RowMessage = { changeId: string; key: string };

const CONTEXT_CHANGED = "context_changed";

function conflictKey(error: ApiError): string {
  const body = error.body as { reason?: unknown } | null;
  return body && body.reason === CONTEXT_CHANGED
    ? "coordinator:changeContextChanged"
    : "coordinator:changeAlreadySettled";
}

/** The plain sentence for a failed Apply or Discard, or null when the row should just refresh. */
function failureKey(error: unknown): string | null {
  if (error instanceof ApiError) {
    if (error.status === 404) return null;
    if (error.status === 409) return conflictKey(error);
  }
  return "coordinator:changeActionFailed";
}

function ChangeRow({
  change,
  canManage,
  disabled,
  message,
  onApply,
  onDiscard,
}: {
  change: PendingChange;
  canManage: boolean;
  disabled: boolean;
  message: string | null;
  onApply: () => void;
  onDiscard: () => void;
}) {
  const { t } = useTranslation();
  return (
    <li className="space-y-2 rounded border p-3" data-testid={`pending-change-${change.id}`}>
      <p className="text-sm font-medium">{change.proposal_title}</p>
      <ContextDiff before={change.base_value} after={change.new_value} />
      {message && (
        <p role="alert" className="text-xs text-destructive" data-testid="pending-change-message">
          {message}
        </p>
      )}
      {canManage && (
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            disabled={disabled}
            onClick={onApply}
            className="cursor-pointer"
            data-testid="pending-change-apply"
          >
            {t("coordinator:changeApply")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={onDiscard}
            className="cursor-pointer"
            data-testid="pending-change-discard"
          >
            {t("coordinator:changeDiscard")}
          </Button>
        </div>
      )}
    </li>
  );
}

function useScrollIntoViewWhenLoaded(ready: boolean) {
  const ref = useRef<HTMLElement | null>(null);
  const searchParams = useSearchParams();
  const wanted = searchParams.get("section") === "autonomy";
  const doneRef = useRef(false);
  useEffect(() => {
    if (!ready || !wanted || doneRef.current) return;
    doneRef.current = true;
    ref.current?.scrollIntoView?.({ block: "start" });
  }, [ready, wanted]);
  return ref;
}

/** The settings list of proposed context changes waiting for Apply or Discard. */
export function ChangesWaiting({ workspaceId, coordinatorId, canManage, onContextApplied }: Props) {
  const { t } = useTranslation();
  const list = usePendingChanges(workspaceId, coordinatorId);
  const save = useSettingsSaveCoordinator();
  const [message, setMessage] = useState<RowMessage | null>(null);
  const sectionRef = useScrollIntoViewWhenLoaded(list.status !== "loading");
  const disabled = save.status === "saving" || save.exclusiveBusy;

  async function decide(change: PendingChange, kind: "apply" | "discard") {
    setMessage(null);
    let failure: unknown = null;
    let settled = false;
    const ran = await save.runExclusive(async () => {
      try {
        if (kind === "discard") {
          await discardPendingChange(workspaceId, coordinatorId, change.id);
          settled = true;
          return;
        }
        await applyPendingChange(workspaceId, coordinatorId, change.id);
        settled = true;
        const fresh = await getCoordinator(workspaceId, coordinatorId);
        onContextApplied?.(fresh.context ?? "");
      } catch (error) {
        failure = error;
      }
    });
    if (!ran) return;
    const key = failure === null ? null : failureKey(failure);
    if (key !== null) setMessage({ changeId: change.id, key });
    const refused =
      failure instanceof ApiError && (failure.status === 404 || failure.status === 409);
    if (failure === null || settled || refused) list.refetch();
  }

  return (
    <section
      ref={sectionRef}
      className="space-y-3"
      aria-labelledby="changes-waiting-heading"
      data-testid="changes-waiting"
    >
      <h3 id="changes-waiting-heading" className="text-sm font-medium">
        {t("coordinator:changesHeading")}
      </h3>
      {list.status === "loading" && (
        <Skeleton className="h-4 w-64" data-testid="changes-waiting-loading" />
      )}
      {list.status === "error" && (
        <div className="space-y-2" data-testid="changes-waiting-error">
          <p className="text-sm text-destructive">{t("coordinator:changesLoadError")}</p>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={list.refetch}>
            {t("coordinator:retry")}
          </Button>
        </div>
      )}
      {list.status === "ready" && list.changes.length === 0 && (
        <p className="text-sm text-muted-foreground" data-testid="changes-waiting-empty">
          {t("coordinator:changesEmpty")}
        </p>
      )}
      {list.status === "ready" && list.changes.length > 0 && (
        <ul className="space-y-3">
          {list.changes.map((change) => (
            <ChangeRow
              key={change.id}
              change={change}
              canManage={canManage}
              disabled={disabled}
              message={message?.changeId === change.id ? t(message.key) : null}
              onApply={() => void decide(change, "apply")}
              onDiscard={() => void decide(change, "discard")}
            />
          ))}
        </ul>
      )}
    </section>
  );
}
