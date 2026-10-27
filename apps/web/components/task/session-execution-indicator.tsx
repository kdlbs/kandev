"use client";

import { useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogTrigger } from "@kandev/ui/dialog";
import { useSessionExecution } from "@/hooks/domains/session/use-session-execution";
import { ExecutionStatusIcon, executionStatusKeys } from "./execution-status-presentation";
import { SessionExecutionDetails } from "./session-execution-details";
import { useAppStore } from "@/components/state-provider";
import { SandboxApprovalNotice } from "./sandbox-approvals";

export function SessionExecutionControl({
  sessionId,
  labelled = false,
  triggerRef,
}: {
  sessionId: string;
  labelled?: boolean;
  triggerRef?: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ownTriggerRef = useRef<HTMLButtonElement>(null);
  const executionRef = triggerRef ?? ownTriggerRef;
  const { session, status } = useSessionExecution(sessionId);
  const approvalsEnabled = useAppStore(
    (state) =>
      state.features.sandboxPolicies && state.auth.mode === "enabled" && state.auth.authenticated,
  );
  const label = t("agents:executionOpen", {
    session: session?.name || sessionId,
    status: t(executionStatusKeys[status]),
  });
  return (
    <>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <Button
            ref={executionRef}
            type="button"
            variant="ghost"
            aria-label={label}
            title={label}
            data-testid={`execution-control-${sessionId}`}
            className={
              labelled
                ? "h-11 min-h-11 min-w-0 max-w-full justify-start gap-2 px-3 cursor-pointer"
                : "h-7 w-7 shrink-0 p-0 cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
            }
            onPointerDown={(event) => {
              event.stopPropagation();
              event.preventDefault();
              event.currentTarget.focus();
            }}
            onClick={(event) => event.stopPropagation()}
            onDoubleClick={(event) => event.stopPropagation()}
            onKeyDown={(event) => event.stopPropagation()}
          >
            <ExecutionStatusIcon status={status} />
            {labelled && (
              <span className="truncate">
                {t("agents:executionStatus")}: {t(executionStatusKeys[status])}
              </span>
            )}
          </Button>
        </DialogTrigger>
        {open && <SessionExecutionDetails sessionId={sessionId} />}
      </Dialog>
      {approvalsEnabled && session?.execution_snapshot?.sandbox && (
        <SandboxApprovalNotice
          sessionId={sessionId}
          sessionName={session.name || sessionId}
          labelled={labelled}
          returnFocusRef={executionRef}
        />
      )}
    </>
  );
}
