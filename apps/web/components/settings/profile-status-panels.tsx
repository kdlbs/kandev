"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  IconAlertCircle,
  IconLoader2,
  IconLock,
  IconPackageOff,
  IconRefresh,
  IconTerminal2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { AgentLoginDialog } from "@/components/settings/agent-login-dialog";
import { AntigravityLoginDialog } from "@/components/settings/antigravity-login-dialog";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import type { AntigravityRuntimeVariant, AvailableAgent } from "@/lib/types/http";

/**
 * Rendered between install completion and the capability probe finishing.
 * Auto-refresh runs on the backend; the UI just signals "still working" so
 * users don't mistake the in-flight probe for a failed install.
 */
export function ProbingPanel() {
  const { t } = useTranslation();
  return (
    <div
      data-testid="profile-probing-panel"
      data-status="probing"
      className="flex items-center gap-3 rounded-md border border-muted bg-muted/40 p-3"
    >
      <IconLoader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
      <p className="text-sm text-muted-foreground">{t("agents:probingCapabilities")}</p>
    </div>
  );
}

function noAuthHint(
  t: TFunction,
  { isAuth, canLogin }: { isAuth: boolean; canLogin: boolean },
): React.ReactNode {
  if (!isAuth) {
    return t("agents:noAuthHintNotInstalled");
  }
  if (canLogin) {
    return t("agents:noAuthHintLogin");
  }
  return t("agents:noAuthHintShell");
}

function noAuthTitle(t: TFunction, isAuth: boolean, antigravity: boolean): string {
  if (!isAuth) return t("agents:notInstalled");
  if (antigravity) return t("agents:antigravityGoogleSignInRequired");
  return t("agents:noAuthLoginRequired");
}

function antigravityNoAuthHint(
  t: TFunction,
  isAuth: boolean,
  runtimeVariant?: AntigravityRuntimeVariant,
): string {
  if (!isAuth) return t("agents:antigravityNotInstalledHint");
  if (runtimeVariant === "cli") return t("agents:antigravityCLISignInHint");
  return t("agents:antigravityStandaloneSignInHint");
}

function NoAuthActions({
  showTerminal,
  onOpenTerminal,
  actionLabel,
  showTerminalIcon,
  onRefresh,
  isLoading,
}: {
  showTerminal: boolean;
  onOpenTerminal: () => void;
  actionLabel?: string;
  showTerminalIcon?: boolean;
  onRefresh: () => Promise<void>;
  isLoading: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="col-span-2 flex w-full items-center gap-2 sm:w-auto sm:shrink-0">
      {showTerminal && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onOpenTerminal}
          className="min-h-11 flex-1 cursor-pointer sm:min-h-9 sm:flex-none"
          data-testid="profile-no-auth-open-terminal"
        >
          {showTerminalIcon && <IconTerminal2 className="mr-2 h-4 w-4" />}
          {actionLabel ?? t("agents:openTerminal")}
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={isLoading}
        className="min-h-11 flex-1 cursor-pointer sm:min-h-9 sm:flex-none"
        data-testid="profile-no-auth-refresh"
      >
        <IconRefresh className={`mr-2 h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
        {t("agents:refresh")}
      </Button>
    </div>
  );
}

type NoAuthPanelProps = {
  agentName: string;
  status: "auth_required" | "not_installed";
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  rawError: string | null;
  runtimeVariant?: AntigravityRuntimeVariant;
};

type NoAuthDialogMode = "login" | "standalone" | "shell" | null;

function noAuthDialogMode(
  agentName: string,
  status: NoAuthPanelProps["status"],
  runtimeVariant: AntigravityRuntimeVariant | undefined,
  hasLoginCommand: boolean,
): NoAuthDialogMode {
  if (status !== "auth_required") return null;
  if (agentName !== "antigravity-acp") return hasLoginCommand ? "login" : "shell";
  if (runtimeVariant === "standalone") return "standalone";
  if (runtimeVariant === "cli" && hasLoginCommand) return "login";
  return null;
}

function NoAuthDialogs({
  mode,
  openMode,
  setOpenMode,
  agentName,
  loginCommand,
  runtimeVariant,
  onRefresh,
}: {
  mode: NoAuthDialogMode;
  openMode: NoAuthDialogMode;
  setOpenMode: (mode: NoAuthDialogMode) => void;
  agentName: string;
  loginCommand?: AvailableAgent["login_command"];
  runtimeVariant?: AntigravityRuntimeVariant;
  onRefresh: () => Promise<void>;
}) {
  switch (mode) {
    case "login":
      return (
        <AgentLoginDialog
          open={openMode === "login"}
          onOpenChange={(open) => setOpenMode(open ? "login" : null)}
          agentName={agentName}
          description={loginCommand?.description}
          command={loginCommand?.cmd}
          variants={loginCommand?.variants}
          runtimeVariant={runtimeVariant}
          onLoginSuccess={() => void onRefresh()}
        />
      );
    case "standalone":
      return (
        <AntigravityLoginDialog
          open={openMode === "standalone"}
          onOpenChange={(open) => setOpenMode(open ? "standalone" : null)}
          onComplete={() => void onRefresh()}
        />
      );
    case "shell":
      return (
        <HostShellDialog
          open={openMode === "shell"}
          onOpenChange={(open) => setOpenMode(open ? "shell" : null)}
          onClose={() => void onRefresh()}
        />
      );
    default:
      return null;
  }
}

export function NoAuthPanel({
  agentName,
  status,
  isLoading,
  onRefresh,
  error,
  rawError,
  runtimeVariant,
}: NoAuthPanelProps) {
  const { t } = useTranslation();
  const antigravity = agentName === "antigravity-acp";
  const isAuth = status === "auth_required";
  const Icon = isAuth ? IconLock : IconPackageOff;
  const title = noAuthTitle(t, isAuth, antigravity);
  const detail = error || rawError;
  const loginCommand = useAppStore((state) =>
    state.availableAgents.items.find((agent) => agent.name === agentName),
  )?.login_command;
  const mode = noAuthDialogMode(agentName, status, runtimeVariant, Boolean(loginCommand));
  const hint = antigravity
    ? antigravityNoAuthHint(t, isAuth, runtimeVariant)
    : noAuthHint(t, { isAuth, canLogin: Boolean(loginCommand) });
  const [openMode, setOpenMode] = useState<NoAuthDialogMode>(null);

  return (
    <div
      data-testid="profile-no-auth-panel"
      data-status={status}
      className="grid grid-cols-[auto,minmax(0,1fr)] gap-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 sm:flex sm:items-start"
    >
      <Icon className="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
      <div className="flex-1 min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium">{title}</p>
          {detail && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="inline-flex items-center gap-1 text-xs font-medium normal-case text-muted-foreground hover:text-foreground cursor-help"
                  data-testid="profile-no-auth-details"
                >
                  <IconAlertCircle className="h-3 w-3" />
                  {t("agents:details")}
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-md">
                <p className="whitespace-pre-wrap break-words text-xs">{detail}</p>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <NoAuthActions
        showTerminal={mode !== null}
        onOpenTerminal={() => setOpenMode(mode)}
        actionLabel={antigravity ? t("agents:signIn") : undefined}
        showTerminalIcon={!antigravity}
        onRefresh={onRefresh}
        isLoading={isLoading}
      />
      {antigravity && !isAuth && (
        <a
          href="https://antigravity.google/docs/cli"
          target="_blank"
          rel="noopener noreferrer"
          className="col-span-2 inline-flex min-h-11 items-center text-sm text-primary underline underline-offset-4"
          data-testid="antigravity-cli-setup-guide"
        >
          {t("agents:antigravityCLISetupGuide")}
        </a>
      )}
      <NoAuthDialogs
        mode={mode}
        openMode={openMode}
        setOpenMode={setOpenMode}
        agentName={agentName}
        loginCommand={loginCommand}
        runtimeVariant={runtimeVariant}
        onRefresh={onRefresh}
      />
    </div>
  );
}
