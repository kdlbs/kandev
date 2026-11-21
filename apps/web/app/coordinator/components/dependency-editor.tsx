"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Skeleton } from "@kandev/ui/skeleton";
import { useTaskDependencies } from "@/hooks/domains/coordinator/use-task-dependencies";
import type { DepRef } from "@/lib/api/domains/coordinator-merge-api";
import { refKey, refValue } from "@/lib/coordinators/merge-dependencies";

type Props = {
  workspaceId: string;
  coordinatorId: string;
  taskId: string;
  canManage: boolean;
};

type DepListProps = {
  refs: DepRef[];
  canManage: boolean;
  onRemove: (ref: DepRef) => void;
};

function DepList({ refs, canManage, onRemove }: DepListProps) {
  const { t } = useTranslation();
  if (refs.length === 0) return <p className="text-sm">{t("coordinator:mergePredecessorsNone")}</p>;
  return (
    <ul className="divide-border divide-y" data-testid="queue-deps-list">
      {refs.map((ref) => (
        <li key={refKey(ref)} className="flex items-center justify-between gap-2 py-1">
          <span className="min-w-0 text-sm break-all">
            {"task" in ref
              ? t("coordinator:mergeDepRefTask", { id: ref.task })
              : t("coordinator:mergeDepRefChange", { key: ref.change })}
          </span>
          {canManage && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="min-h-11 cursor-pointer sm:min-h-0"
              aria-label={t("coordinator:mergePredecessorsRemove", { ref: refValue(ref) })}
              onClick={() => onRemove(ref)}
            >
              {t("coordinator:mergePredecessorsRemoveAction")}
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}

/** A card's predecessor set: add, remove and save, refusing duplicates and a stale revision. */
export function DependencyEditor({ workspaceId, coordinatorId, taskId, canManage }: Props) {
  const { t } = useTranslation();
  const deps = useTaskDependencies(workspaceId, coordinatorId, taskId, true);
  const [input, setInput] = useState("");

  if (deps.status === "error") {
    return (
      <div className="space-y-2" data-testid="queue-deps-error">
        <p className="text-destructive text-sm">{t("coordinator:mergeDepLoadFailed")}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-0"
          onClick={deps.retry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (deps.status !== "ready") {
    return <Skeleton className="h-4 w-48" data-testid="queue-deps-loading" />;
  }

  return (
    <div className="space-y-3" data-testid={`queue-deps-${taskId}`}>
      <p className="text-muted-foreground text-xs">{t("coordinator:mergePredecessorsHelp")}</p>
      <DepList refs={deps.refs} canManage={canManage} onRemove={deps.remove} />
      {canManage && (
        <form
          className="flex flex-col gap-2 sm:flex-row"
          onSubmit={(event) => {
            event.preventDefault();
            if (deps.add(input)) setInput("");
          }}
        >
          <Input
            value={input}
            onChange={(event) => setInput(event.target.value)}
            aria-label={t("coordinator:mergePredecessorsAddLabel")}
            placeholder={t("coordinator:mergePredecessorsAddPlaceholder")}
            className="min-h-11 sm:min-h-0"
            data-testid="queue-deps-input"
          />
          <Button
            type="submit"
            variant="outline"
            className="min-h-11 cursor-pointer sm:min-h-0"
            data-testid="queue-deps-add"
          >
            {t("coordinator:mergePredecessorsAdd")}
          </Button>
        </form>
      )}
      {deps.errorKey && (
        <p role="alert" className="text-destructive text-sm" data-testid="queue-deps-message">
          {t(deps.errorKey)}
        </p>
      )}
      {deps.saved && !deps.errorKey && (
        <p role="status" className="text-sm" data-testid="queue-deps-saved">
          {t("coordinator:mergeDepSaved")}
        </p>
      )}
      {canManage && (
        <Button
          type="button"
          className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
          disabled={!deps.dirty || deps.saving}
          onClick={() => void deps.save()}
          data-testid="queue-deps-save"
        >
          {t("coordinator:mergePredecessorsSave")}
        </Button>
      )}
    </div>
  );
}
