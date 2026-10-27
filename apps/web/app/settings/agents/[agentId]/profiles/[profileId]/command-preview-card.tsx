"use client";

import { useEffect, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  previewAgentCommandAction,
  type CommandPreviewRequest,
  type CommandPreviewResponse,
} from "@/app/actions/agents";
import { ApiError } from "@/lib/api/client";
import type { CLIFlag } from "@/lib/types/http";
import type { CommandSummary } from "@/lib/types/execution";
import { useSettingsTargetRegistration } from "@/components/settings/settings-target-provider";

type CommandPreviewCardProps = {
  discoveryTargetId?: string;
  agentName: string;
  model: string;
  permissionSettings: Record<string, boolean>;
  cliPassthrough: boolean;
  cliFlags: CLIFlag[];
  commandPrefix?: string;
};

function useCommandPreview(agentName: string, request: CommandPreviewRequest, open: boolean) {
  const [preview, setPreview] = useState<CommandPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<"invalid" | "unavailable" | null>(null);
  const latestRequest = useRef(0);
  const key = JSON.stringify(request);
  useEffect(() => {
    const sequence = ++latestRequest.current;
    if (!open) return;
    setPreview(null);
    setError(null);
    setLoading(true);
    const timer = setTimeout(async () => {
      try {
        const result = await previewAgentCommandAction(agentName, JSON.parse(key));
        if (sequence !== latestRequest.current) return;
        if (!result.composition) {
          setError("unavailable");
          return;
        }
        setPreview(result);
      } catch (cause) {
        if (sequence !== latestRequest.current) return;
        setError(cause instanceof ApiError && cause.status === 400 ? "invalid" : "unavailable");
      } finally {
        if (sequence === latestRequest.current) setLoading(false);
      }
    }, 300);
    return () => {
      clearTimeout(timer);
      latestRequest.current++;
    };
  }, [agentName, key, open]);
  return { preview, loading, error };
}

function CommandIdentity({ label, command }: { label: string; command: CommandSummary }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="flex flex-wrap items-center gap-2 text-sm">
        <code className="break-all">{command.executable || t("common:unknown")}</code>
        {command.references.map((reference, index) => (
          <code key={`${index}:${reference}`} className="break-all text-xs">
            {reference}
          </code>
        ))}
        {command.omitted_arguments > 0 && (
          <span className="text-xs text-muted-foreground">{t("agents:argumentsOmitted")}</span>
        )}
      </dd>
    </div>
  );
}

export function CommandPreviewCard({
  discoveryTargetId,
  agentName,
  model,
  permissionSettings,
  cliPassthrough,
  cliFlags,
  commandPrefix,
}: CommandPreviewCardProps) {
  const { t } = useTranslation();
  const registerTarget = useSettingsTargetRegistration(discoveryTargetId);
  const [open, setOpen] = useState(false);
  const { preview, loading, error } = useCommandPreview(
    agentName,
    {
      model,
      permission_settings: permissionSettings,
      cli_passthrough: cliPassthrough,
      cli_flags: cliFlags,
      command_prefix: commandPrefix,
      safe_only: true,
    },
    open,
  );
  return (
    <details
      data-testid="execution-command-preview"
      className="rounded-md border px-3"
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary
        ref={registerTarget}
        className="min-h-11 cursor-pointer content-center text-sm font-medium"
      >
        {t("agents:commandPreview")}
      </summary>
      <div className="space-y-3 pb-3" aria-live="polite" aria-busy={loading}>
        <p className="text-xs text-muted-foreground">{t("agents:commandCompositionHelp")}</p>
        {loading && <p className="text-xs text-muted-foreground">{t("common:loading")}</p>}
        {error && (
          <p role="alert" className="text-xs text-destructive">
            {t(
              error === "invalid"
                ? "agents:invalidCommandWrapper"
                : "agents:failedToLoadCommandPreview",
            )}
          </p>
        )}
        {!loading && !error && preview?.composition && (
          <dl className="space-y-3">
            {preview.composition.wrapper && (
              <CommandIdentity
                label={t("agents:commandPrefix")}
                command={preview.composition.wrapper.command}
              />
            )}
            <CommandIdentity label={t("agents:agentCommand")} command={preview.composition.agent} />
          </dl>
        )}
        {!loading && !error && preview?.flag_destination === "acp_bridge" && (
          <p
            data-testid="command-preview-flag-destination"
            className="text-xs text-muted-foreground"
          >
            {t("agents:cliFlagsGoToBridge")}
          </p>
        )}
      </div>
    </details>
  );
}
