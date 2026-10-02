"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { managedRuntimeStartupCopy } from "../managed-runtime-startup-copy";
import { ActionButtons } from "./action-message-actions";
import { ActionMessageDetails, type ActionMeta } from "./action-message-details";

export function ManagedRuntimeStartupRecoveryMessage({
  metadata,
  taskId,
  onRecoveryRequested,
}: {
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
}) {
  const { t } = useTranslation();
  const copy = managedRuntimeStartupCopy(metadata, t);
  const actions = metadata.actions?.slice(0, 1) ?? [];
  return (
    <section
      data-testid="managed-runtime-startup-recovery"
      role="alert"
      className="w-full min-w-0 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:p-4"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">{copy.title}</h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{copy.summary}</p>
          <ActionMessageDetails metadata={metadata} />
          {actions.length > 0 && (
            <ActionButtons
              actions={actions}
              taskId={taskId}
              onRecoveryRequested={onRecoveryRequested}
              labelOverride={t("chat:managedRuntimeRetry")}
            />
          )}
        </div>
      </div>
    </section>
  );
}
