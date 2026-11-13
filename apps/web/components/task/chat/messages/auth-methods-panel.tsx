"use client";

import { useState, useCallback } from "react";
import { IconTerminal2, IconCopy, IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { RecoveryAuthMethod } from "@/components/task/chat/types";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useTranslation } from "react-i18next";

function buildFullCommand(termAuth: RecoveryAuthMethod["terminal_auth"]): string | null {
  if (!termAuth) return null;
  if (termAuth.args && termAuth.args.length > 0) {
    return `${termAuth.command} ${termAuth.args.join(" ")}`;
  }
  return termAuth.command;
}

export function AuthMethodsPanel({
  methods,
  executionHostGuidance = false,
}: {
  methods: RecoveryAuthMethod[];
  executionHostGuidance?: boolean;
}) {
  const { t } = useTranslation();
  const terminalGuidanceRequired =
    executionHostGuidance || methods.some((method) => method.terminal_auth !== undefined);
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        {terminalGuidanceRequired
          ? t("task:authenticationRequiredOnExecutionHost")
          : t("task:authenticationRequiredLogInBeforeResuming")}
      </div>
      {methods.map((method) => (
        <AuthMethodRow
          key={method.id}
          method={method}
          executionHostGuidance={terminalGuidanceRequired}
        />
      ))}
    </div>
  );
}

export function GenericAuthPanel({
  onOpenTerminal,
  allowTerminal = true,
  executionHostGuidance = false,
}: {
  onOpenTerminal: () => void;
  allowTerminal?: boolean;
  executionHostGuidance?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        {executionHostGuidance
          ? t("task:authenticationRequiredOnExecutionHost")
          : t("task:authenticationRequiredPleaseLogInVia")}
      </div>
      {allowTerminal && (
        <Button
          variant="outline"
          size="sm"
          className={controlSizingClassName("standard", "text-[11px] cursor-pointer gap-1 px-2")}
          onClick={onOpenTerminal}
        >
          <IconTerminal2 className="h-3 w-3" />
          {t("task:openTerminal")}
        </Button>
      )}
    </div>
  );
}

function AuthMethodRow({
  method,
  executionHostGuidance,
}: {
  method: RecoveryAuthMethod;
  executionHostGuidance: boolean;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const termAuth = method.terminal_auth;
  const fullCommand = buildFullCommand(termAuth);

  const handleCopy = useCallback(async () => {
    if (!fullCommand) return;
    if (await copyToClipboard(fullCommand)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [fullCommand]);

  return (
    <div className="space-y-1">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-xs text-muted-foreground">
        <span className="wrap-anywhere">{method.name}</span>
        <code className="break-all text-[10px]">{method.id}</code>
      </div>
      {termAuth && fullCommand ? (
        <div className="flex items-center gap-1.5">
          <div className="flex items-center gap-1.5 text-xs bg-muted/50 rounded px-2 py-1 font-mono flex-1 min-w-0">
            <IconTerminal2 className="h-3 w-3 shrink-0 text-muted-foreground" />
            <code className="text-[11px] truncate">{fullCommand}</code>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className={controlSizingClassName("icon", "p-0 cursor-pointer shrink-0")}
                onClick={handleCopy}
                aria-label={copied ? t("task:commandCopied") : t("task:copyCommand")}
              >
                {copied ? (
                  <IconCheck className="h-3 w-3 text-green-500" />
                ) : (
                  <IconCopy className="h-3 w-3 text-muted-foreground" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">{t("task:copyCommand")}</TooltipContent>
          </Tooltip>
        </div>
      ) : (
        <div className="wrap-anywhere text-xs text-muted-foreground">
          {method.description ||
            (executionHostGuidance
              ? t("task:authenticationRequiredOnExecutionHost")
              : t("task:loginRequired"))}
        </div>
      )}
    </div>
  );
}
