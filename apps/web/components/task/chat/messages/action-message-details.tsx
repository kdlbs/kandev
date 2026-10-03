import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { AuthMethodsPanel, GenericAuthPanel } from "./auth-methods-panel";
import { RemediationLink } from "@/components/task/remediation-link";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { MessageAction, RecoveryAuthMethod } from "@/components/task/chat/types";
import type { AgentErrorCause } from "@/lib/types/task-status-summary";
import type { SessionErrorDetailsField } from "@/lib/session-error-details";

export type ActionMeta = {
  actions?: MessageAction[];
  action_visibility?: "running";
  variant?: string;
  recovery_actions?: boolean;
  scope?: "session" | "task";
  error_stamp?: string;
  recovery_stamp?: string;
  is_auth_error?: boolean;
  auth_methods?: RecoveryAuthMethod[];
  error_output?: string;
  failure_kind?: string;
  missing_branch?: string;
  provider_name?: string;
  model_id?: string;
  reset_at?: string;
  remediation_url?: string;
  retrying?: boolean;
  recovery_mode?: string;
  recovery_phase?: string;
  recovery_disposition?: string;
  recovery_reason?: string;
  attempts_started?: number;
  runtime_retained?: boolean;
  provider_error?: {
    source?: string;
    provider_id?: string;
    error_kind?: string;
    rpc_code?: number;
  };
  attempt?: number;
  max_attempts?: number;
  retry_in_seconds?: number;
  retry_at?: string;
  failure_code?: string;
  code?: string;
  causes?: AgentErrorCause[];
  occurred_at?: string;
  execution_id?: string;
  failure_details?: string;
  startup_reason?: string;
  startup_attempts?: number;
  startup_npm_code?: string;
  attempt_id?: string;
  phase?: string;
};

export function TechnicalDetails({
  children,
  structuredFields,
}: {
  children: string;
  structuredFields?: readonly SessionErrorDetailsField[];
}) {
  return <SessionErrorDetails structuredFields={structuredFields}>{children}</SessionErrorDetails>;
}

export function ActionMessageDetails({
  metadata,
  technicalDetails,
}: {
  metadata: ActionMeta | undefined;
  technicalDetails?: string;
}) {
  const [hostShellOpen, setHostShellOpen] = useState(false);
  const [hostShellCommand, setHostShellCommand] = useState<string | undefined>(undefined);
  const { t } = useTranslation();

  const openHostShellWithCommand = useCallback((command: string) => {
    setHostShellCommand(command + "\n");
    setHostShellOpen(true);
  }, []);
  const openHostShell = useCallback(() => {
    setHostShellCommand(undefined);
    setHostShellOpen(true);
  }, []);

  if (!metadata) return null;
  const errorOutput = metadata.error_output || technicalDetails;
  const structuredFields = providerErrorDetails(metadata.provider_error, t);
  return (
    <>
      {metadata.remediation_url && <RemediationLink url={metadata.remediation_url} />}
      {(errorOutput || structuredFields.length > 0) && (
        <TechnicalDetails structuredFields={structuredFields}>{errorOutput ?? ""}</TechnicalDetails>
      )}
      {metadata.is_auth_error && metadata.auth_methods && metadata.auth_methods.length > 0 && (
        <AuthMethodsPanel
          methods={metadata.auth_methods}
          onOpenTerminal={openHostShellWithCommand}
        />
      )}
      {metadata.is_auth_error && (!metadata.auth_methods || metadata.auth_methods.length === 0) && (
        <GenericAuthPanel onOpenTerminal={openHostShell} />
      )}
      <HostShellDialog
        open={hostShellOpen}
        onOpenChange={setHostShellOpen}
        initialInput={hostShellCommand}
      />
    </>
  );
}

function providerErrorDetails(
  providerError: ActionMeta["provider_error"],
  translate: ReturnType<typeof useTranslation>["t"],
): SessionErrorDetailsField[] {
  if (!providerError) return [];
  const fields: SessionErrorDetailsField[] = [];
  if (providerError.source) {
    fields.push({ label: translate("chat:providerErrorSource"), value: providerError.source });
  }
  if (providerError.provider_id) {
    fields.push({
      label: translate("chat:providerErrorProvider"),
      value: providerError.provider_id,
    });
  }
  if (providerError.error_kind) {
    fields.push({ label: translate("chat:providerErrorKind"), value: providerError.error_kind });
  }
  if (Number.isInteger(providerError.rpc_code) && providerError.rpc_code !== 0) {
    fields.push({
      label: translate("chat:providerErrorRpcCode"),
      value: String(providerError.rpc_code),
    });
  }
  return fields;
}
