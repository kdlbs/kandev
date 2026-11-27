"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { PluginCapabilityApprovalAudit } from "./plugin-capability-approval-audit";
import { CapabilityGroup, ContextValue } from "./plugin-capability-approval-fields";
import { usePluginCapabilityApproval } from "./use-plugin-capability-approval";

type ApprovalEditorProps = {
  workspaceName: string;
  approval: ReturnType<typeof usePluginCapabilityApproval>;
};

export function PluginCapabilityApprovalEditor({ workspaceName, approval }: ApprovalEditorProps) {
  if (!approval.context) return null;

  return (
    <div className="space-y-4" data-testid="plugin-approval-context">
      <ApprovalContextDetails workspaceName={workspaceName} approval={approval} />
      <ApprovalActions approval={approval} />
      <PluginCapabilityApprovalAudit events={approval.context.audit_events} />
    </div>
  );
}

function ApprovalContextDetails({ workspaceName, approval }: ApprovalEditorProps) {
  const { t } = useTranslation();
  const context = approval.context;
  if (!context) return null;
  const reads = context.declared_capability_ids.filter((id) => id.startsWith("host.v2.read:"));
  const writes = context.declared_capability_ids.filter((id) => id.startsWith("host.v2.write:"));

  return (
    <>
      <div className="grid gap-2 rounded-md border border-border/70 bg-muted/20 p-3 text-sm sm:grid-cols-2">
        <ContextValue label={t("plugins:approvalInstallation")} value={context.installation_id} />
        <ContextValue
          label={t("plugins:approvalSelectedWorkspace")}
          value={`${workspaceName} (${context.workspace_id})`}
        />
        <ContextValue
          label={t("plugins:approvalRevision")}
          value={String(context.approval?.revision ?? 0)}
        />
      </div>
      {context.requires_review && (
        <p
          className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400"
          data-testid="plugin-approval-upgrade-review"
        >
          {t("plugins:capabilityApprovalUpgradeReview")}
        </p>
      )}
      {context.approval?.state === "revoked" && (
        <p className="text-sm text-muted-foreground" data-testid="plugin-approval-revoked">
          {t("plugins:capabilityApprovalRevokedState")}
        </p>
      )}
      <div className="space-y-3" data-testid="plugin-requested-capabilities">
        <CapabilityGroup
          title={t("plugins:readCapabilities")}
          capabilities={reads}
          selected={approval.selectedCapabilityIds}
          disabled={approval.saving || approval.revoking}
          onToggle={approval.toggleCapability}
        />
        <CapabilityGroup
          title={t("plugins:writeCapabilities")}
          capabilities={writes}
          selected={approval.selectedCapabilityIds}
          disabled={approval.saving || approval.revoking}
          onToggle={approval.toggleCapability}
        />
        {context.declared_capability_ids.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("plugins:noDeclaredCapabilities")}</p>
        )}
      </div>
      <div className="space-y-2">
        <Label htmlFor="plugin-approval-reason">{t("plugins:approvalReason")}</Label>
        <Textarea
          id="plugin-approval-reason"
          data-testid="plugin-approval-reason"
          value={approval.reason}
          onChange={(event) => approval.setReason(event.target.value)}
          placeholder={t("plugins:approvalReasonPlaceholder")}
          disabled={approval.saving || approval.revoking}
          maxLength={300}
          className="max-sm:min-h-20"
        />
      </div>
      {approval.errorKey && (
        <p className="text-sm text-destructive" role="alert" data-testid="plugin-approval-error">
          {t(approval.errorKey)}
        </p>
      )}
    </>
  );
}

function ApprovalActions({
  approval,
}: {
  approval: ReturnType<typeof usePluginCapabilityApproval>;
}) {
  const { t } = useTranslation();
  const hasActiveApproval = approval.context?.approval?.state === "active";
  const touchTargets = "cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";
  return (
    <div className="sticky bottom-0 z-10 flex flex-col gap-2 border-t bg-card/95 py-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)] backdrop-blur sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          data-testid="plugin-approval-save"
          className={controlSizingClassName("standard", touchTargets)}
          disabled={!approval.canSave}
          onClick={() => void approval.save()}
        >
          {t("plugins:saveCapabilityApproval")}
        </Button>
        {hasActiveApproval && (
          <Button
            data-testid="plugin-approval-revoke"
            variant="outline"
            className={controlSizingClassName("standard", touchTargets)}
            disabled={!approval.canRevoke || approval.saving || approval.revoking}
            onClick={() => void approval.revoke()}
          >
            {t("plugins:revokeCapabilityApproval")}
          </Button>
        )}
      </div>
      <span
        role="status"
        aria-live="polite"
        className="text-sm text-muted-foreground"
        data-testid="plugin-approval-status"
      >
        {approval.status}
      </span>
    </div>
  );
}
