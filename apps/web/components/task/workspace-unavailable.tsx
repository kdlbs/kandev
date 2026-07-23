"use client";

import { IconAlertCircle, IconChevronDown } from "@tabler/icons-react";

type WorkspaceUnavailableProps = {
  error?: string | null;
};

export function WorkspaceUnavailable({ error }: WorkspaceUnavailableProps) {
  return (
    <div
      data-testid="workspace-unavailable"
      role="status"
      aria-label="Workspace unavailable"
      className="h-full w-full min-w-0 p-4"
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconAlertCircle
          className="mt-0.5 h-4 w-4 flex-shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium text-foreground">Workspace unavailable</div>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            This session did not finish setting up its workspace.
          </p>
          {error && (
            <details className="mt-2 min-w-0 text-xs text-muted-foreground">
              <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 sm:min-h-8">
                <IconChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
                Technical details
              </summary>
              <pre className="max-h-48 max-w-full overflow-y-auto overscroll-contain whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px]">
                {error}
              </pre>
            </details>
          )}
        </div>
      </div>
    </div>
  );
}
