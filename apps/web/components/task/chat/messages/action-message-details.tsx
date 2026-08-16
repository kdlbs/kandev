import { useCallback, useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { AuthMethodsPanel, GenericAuthPanel } from "./auth-methods-panel";
import { RemediationLink } from "@/components/task/remediation-link";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { MessageAction, RecoveryAuthMethod } from "@/components/task/chat/types";

export type ActionMeta = {
  actions?: MessageAction[];
  action_visibility?: "running";
  variant?: string;
  recovery_actions?: boolean;
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
  attempt?: number;
  max_attempts?: number;
  retry_in_seconds?: number;
  retry_at?: string;
  failure_code?: string;
  failure_details?: string;
};

export function TechnicalDetails({ children }: { children: string }) {
  const { t } = useTranslation();
  return (
    <details className="mt-2 min-w-0 text-xs text-muted-foreground">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 sm:min-h-8">
        <IconChevronDown className="h-3.5 w-3.5" />
        {t("chat:technicalDetails")}
      </summary>
      <pre className="max-h-[300px] max-w-full overflow-y-auto whitespace-pre-wrap break-words rounded bg-muted/50 p-2 font-mono text-[11px]">
        {children}
      </pre>
    </details>
  );
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
  return (
    <>
      {metadata.remediation_url && <RemediationLink url={metadata.remediation_url} />}
      {errorOutput && <TechnicalDetails>{errorOutput}</TechnicalDetails>}
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
