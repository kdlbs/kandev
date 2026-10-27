"use client";

import { useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogTrigger } from "@kandev/ui/dialog";
import { useSandboxApprovals } from "@/hooks/domains/session/use-sandbox-approvals";
import type { SandboxApproval } from "@/lib/api/domains/sandbox-approvals-api";
import { SandboxApprovalReview, sandboxApprovalState } from "./sandbox-approval-review";

export function SandboxApprovals({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const model = useSandboxApprovals(sessionId);
  if (model.isPending) return <p role="status">{t("common:loading")}</p>;
  if (!model.isAvailable) return <p>{t("agents:sandboxApprovalUnavailable")}</p>;
  if (model.approvals.length === 0) return <p>{t("agents:sandboxApprovalEmpty")}</p>;
  return (
    <ul className="space-y-3">
      {model.approvals.map((approval) => (
        <li key={approval.request_id} className="space-y-2">
          <p className="break-all">{approval.target}</p>
          <p>{t(`agents:sandboxApprovalState_${sandboxApprovalState(approval)}`)}</p>
          <ApprovalReviewButton sessionId={sessionId} approval={approval} />
        </li>
      ))}
    </ul>
  );
}

function ApprovalReviewButton({
  sessionId,
  approval,
}: {
  sessionId: string;
  approval: SandboxApproval;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="h-7 max-md:h-11 [@media(pointer:coarse)]:h-11 cursor-pointer"
        >
          {t("agents:sandboxApprovalReview")}
        </Button>
      </DialogTrigger>
      {open && <SandboxApprovalReview sessionId={sessionId} requestId={approval.request_id} />}
    </Dialog>
  );
}

export function SandboxApprovalNotice({
  sessionId,
  sessionName,
  labelled,
  returnFocusRef,
}: {
  sessionId: string;
  sessionName: string;
  labelled: boolean;
  returnFocusRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const model = useSandboxApprovals(sessionId);
  const [requestId, setRequestId] = useState<string | null>(null);
  const pending = model.approvals.filter(
    (approval) => sandboxApprovalState(approval) === "pending",
  );
  if (pending.length === 0 && !requestId) return null;
  const label = t("agents:sandboxApprovalNoticeLabel", { session: sessionName });
  return (
    <Dialog
      open={Boolean(requestId)}
      onOpenChange={(open) => {
        if (!open) setRequestId(null);
      }}
    >
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          aria-label={label}
          title={label}
          className={
            labelled
              ? "h-11 min-h-11 min-w-0 gap-2 px-3 text-amber-600 dark:text-amber-400 cursor-pointer"
              : "h-7 w-7 shrink-0 p-0 text-amber-600 dark:text-amber-400 cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          }
          onPointerDown={(event) => {
            event.stopPropagation();
            event.preventDefault();
            event.currentTarget.focus();
          }}
          onClick={(event) => {
            event.stopPropagation();
            if (pending[0]) setRequestId(pending[0].request_id);
          }}
          onDoubleClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <IconAlertTriangle aria-hidden="true" className="size-4 shrink-0" />
          {labelled && (
            <span className="truncate">
              {t("agents:sandboxApprovalPending", { count: pending.length })}
            </span>
          )}
        </Button>
      </DialogTrigger>
      {requestId && (
        <SandboxApprovalReview
          sessionId={sessionId}
          requestId={requestId}
          onCloseAutoFocus={(event) => {
            if (pending.length === 0) {
              event.preventDefault();
              returnFocusRef.current?.focus();
            }
          }}
        />
      )}
    </Dialog>
  );
}
