"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { ActionButtons } from "./action-message-actions";
import { ActionMessageDetails, type ActionMeta } from "./action-message-details";

export function ManagedRuntimeRecoveryMessageShell({
  testId,
  title,
  summary,
  metadata,
  taskId,
  onRecoveryRequested,
  retryLabel,
}: {
  testId: string;
  title: string;
  summary: string;
  metadata: ActionMeta;
  taskId?: string;
  onRecoveryRequested: () => void;
  retryLabel: string;
}) {
  const actions = metadata.actions?.slice(0, 1) ?? [];
  return (
    <section
      data-testid={testId}
      role="alert"
      className="w-full min-w-0 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:p-4"
    >
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium text-foreground">{title}</h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{summary}</p>
          <ActionMessageDetails metadata={metadata} />
          {actions.length > 0 && (
            <ActionButtons
              actions={actions}
              taskId={taskId}
              onRecoveryRequested={onRecoveryRequested}
              labelOverride={retryLabel}
            />
          )}
        </div>
      </div>
    </section>
  );
}
