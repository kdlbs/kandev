"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import {
  IconPlayerPlay,
  IconPlus,
  IconFolder,
  IconRefresh,
  IconGitBranch,
  IconLoader2,
} from "@tabler/icons-react";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { cn } from "@kandev/ui/lib/utils";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  selectPrimaryRecoveryAction,
  type RecoveryActionKind,
} from "@/lib/session-recovery-actions";

export type RecoveryChoice = {
  kind: RecoveryActionKind;
  label: string;
  onClick: () => void;
  testId?: string;
  disabled?: boolean;
  tooltip?: string;
};

export function RecoveryActions({
  actions,
  busy = false,
  blocked = false,
  preferred,
  busyAction,
}: {
  actions: RecoveryChoice[];
  busy?: boolean;
  blocked?: boolean;
  preferred?: RecoveryActionKind;
  busyAction?: RecoveryActionKind | null;
}) {
  const { t } = useTranslation();
  const warningId = useId();
  const warnings = [
    ...new Set(
      actions.flatMap((action) =>
        action.tooltip ? [sanitizeSessionErrorDetails(action.tooltip)] : [],
      ),
    ),
  ].filter(Boolean);
  const primaryKind = selectPrimaryRecoveryAction(
    actions.filter((action) => !action.disabled).map((action) => action.kind),
    blocked,
  );
  const primary = actions.find(
    (action) => action.kind === (preferred ?? primaryKind) && !action.disabled,
  );
  if (blocked || !actions.length) return null;
  const ordered = primary ? [primary, ...actions.filter((action) => action !== primary)] : actions;
  return (
    <div>
      <div
        className="mt-3 flex min-w-0 flex-col gap-2 md:flex-row md:flex-wrap md:items-center"
        aria-busy={busy || undefined}
      >
        {ordered.map((action) => (
          <Button
            key={action.kind}
            type="button"
            variant="outline"
            aria-label={action.label}
            aria-describedby={action.tooltip ? warningId : undefined}
            title={action.tooltip ? sanitizeSessionErrorDetails(action.tooltip) : undefined}
            data-recommended={action === primary}
            disabled={busy || action.disabled}
            onClick={action.onClick}
            data-testid={action.testId}
            className={controlSizingClassName(
              "standard",
              "h-auto min-h-7 w-full cursor-pointer gap-1.5 whitespace-normal py-0.5 md:w-auto",
            )}
          >
            <RecoveryActionIcon kind={action.kind} />
            {action.label}
          </Button>
        ))}
      </div>
      {warnings.length > 0 && (
        <p id={warningId} className="mt-2 wrap-anywhere text-xs text-muted-foreground">
          {warnings.join(" ")}
        </p>
      )}
      <div
        role="status"
        className={cn(
          "flex items-center gap-2 text-xs text-muted-foreground",
          busy && "mt-2 min-h-5",
        )}
      >
        {busy && (
          <>
            <IconLoader2
              aria-hidden="true"
              className="size-3.5 animate-spin motion-reduce:animate-none"
            />
            {busyAction ? pendingLabel(busyAction, t) : t("task:workflowMovePreviewChecking")}
          </>
        )}
      </div>
    </div>
  );
}

function RecoveryActionIcon({ kind }: { kind: RecoveryActionKind }) {
  const icons = {
    fresh_start: IconPlus,
    restore: IconFolder,
    runtime_retry: IconRefresh,
    resume_new_branch: IconGitBranch,
    resume: IconPlayerPlay,
  };
  const Icon = icons[kind];
  return <Icon aria-hidden="true" className="size-3.5 shrink-0" />;
}

function pendingLabel(action: RecoveryActionKind, t: ReturnType<typeof useTranslation>["t"]) {
  if (action === "restore") return t("task:restoring");
  if (action === "fresh_start") return t("task:starting");
  if (action === "runtime_retry") return t("task:retrying");
  return t("task:resuming");
}
