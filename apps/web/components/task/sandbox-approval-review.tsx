"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { DialogClose, DialogContent, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSandboxApprovals } from "@/hooks/domains/session/use-sandbox-approvals";
import type { SandboxApproval } from "@/lib/api/domains/sandbox-approvals-api";

export function SandboxApprovalReview({
  sessionId,
  requestId,
  onCloseAutoFocus,
}: {
  sessionId: string;
  requestId: string;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const model = useSandboxApprovals(sessionId);
  const approval = model.approvals.find((row) => row.request_id === requestId);
  return (
    <DialogContent
      showCloseButton={false}
      enterConfirms={false}
      onCloseAutoFocus={onCloseAutoFocus}
      className={
        isMobile
          ? "inset-0 h-dvh max-h-dvh w-full max-w-none sm:max-w-none translate-x-0 translate-y-0 rounded-none border-0 flex flex-col overflow-hidden pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
          : "flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-xl"
      }
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <div className="flex shrink-0 items-start gap-3">
        {isMobile && (
          <DialogClose asChild>
            <Button type="button" variant="ghost" className="h-12 min-h-12 px-3 cursor-pointer">
              <IconArrowLeft aria-hidden="true" className="size-4" />
              {t("common:back")}
            </Button>
          </DialogClose>
        )}
        <div className="min-w-0 flex-1">
          <DialogTitle>{t("agents:sandboxApprovalTitle")}</DialogTitle>
          <DialogDescription className="mt-2">{t("agents:sandboxApprovalHelp")}</DialogDescription>
        </div>
        {!isMobile && (
          <DialogClose asChild>
            <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common:close")}>
              <IconX aria-hidden="true" className="size-4" />
            </Button>
          </DialogClose>
        )}
      </div>
      <div
        className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-1 py-3 text-sm"
        data-testid="sandbox-approval-body"
      >
        {model.isPending && <p role="status">{t("common:loading")}</p>}
        {approval ? (
          <ApprovalOperationDetails approval={approval} snapshot={model.snapshot} />
        ) : (
          <p>{t("agents:sandboxApprovalUnavailable")}</p>
        )}
        {model.failedRequestId === requestId && (
          <p role="alert">{t("agents:sandboxApprovalFailure")}</p>
        )}
      </div>
      {approval && <ApprovalDecisionActions approval={approval} model={model} />}
    </DialogContent>
  );
}

export function sandboxApprovalState(approval: SandboxApproval) {
  return approval.state === "pending" && Date.parse(approval.expires_at) <= Date.now()
    ? "expired"
    : approval.state;
}

function ApprovalOperationDetails({
  approval,
  snapshot,
}: {
  approval: SandboxApproval;
  snapshot: ReturnType<typeof useSandboxApprovals>["snapshot"];
}) {
  const { t } = useTranslation();
  return (
    <>
      <p role="status">{t(`agents:sandboxApprovalState_${sandboxApprovalState(approval)}`)}</p>
      <dl className="space-y-3">
        <div>
          <dt className="text-muted-foreground">{t("task:session")}</dt>
          <dd className="break-all">{approval.session_id}</dd>
        </div>
        {snapshot?.sandbox && (
          <div>
            <dt className="text-muted-foreground">{t("agents:executionPolicy")}</dt>
            <dd className="break-words">
              {snapshot.sandbox.policy_name} ({snapshot.sandbox.revision_number})
            </dd>
          </div>
        )}
      </dl>
      <div>
        <p className="font-medium">
          {t(
            approval.access === "read"
              ? "agents:sandboxApprovalRead"
              : "agents:sandboxApprovalWrite",
          )}
        </p>
        <p className="break-all font-mono mt-1">{approval.target}</p>
      </div>
      <p>{t("agents:sandboxApprovalDeadline", { time: approval.expires_at })}</p>
      <div>
        <p className="text-muted-foreground">{t("agents:executionPolicyDigest")}</p>
        <p className="mt-1 break-all font-mono">{approval.policy_digest}</p>
      </div>
    </>
  );
}

function ApprovalDecisionActions({
  approval,
  model,
}: {
  approval: SandboxApproval;
  model: ReturnType<typeof useSandboxApprovals>;
}) {
  const { t } = useTranslation();
  const allowed = model.canDecide(approval);
  return (
    <div className="shrink-0 space-y-3 border-t pt-4">
      {!allowed && (
        <p className="text-xs text-muted-foreground">{t("agents:sandboxApprovalReadOnly")}</p>
      )}
      <div className="flex gap-3">
        <Button
          type="button"
          variant="outline"
          className="h-7 max-md:h-11 [@media(pointer:coarse)]:h-11 flex-1 cursor-pointer"
          disabled={!allowed}
          onClick={() => void model.decide(approval, "deny")}
        >
          {t("task:deny")}
        </Button>
        <Button
          type="button"
          className="h-7 max-md:h-11 [@media(pointer:coarse)]:h-11 flex-1 cursor-pointer"
          disabled={!allowed}
          onClick={() => void model.decide(approval, "allow_once")}
        >
          {t("agents:sandboxApprovalAllowOnce")}
        </Button>
      </div>
    </div>
  );
}
