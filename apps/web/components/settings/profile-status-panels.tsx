"use client";

import { useState } from "react";
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
import { HostShellDialog } from "@/components/settings/host-shell-dialog";

/**
 * Rendered between install completion and the capability probe finishing.
 * Auto-refresh runs on the backend; the UI just signals "still working" so
 * users don't mistake the in-flight probe for a failed install.
 */
export function ProbingPanel() {
  return (
    <div
      data-testid="profile-probing-panel"
      data-status="probing"
      className="flex items-center gap-3 rounded-md border border-muted bg-muted/40 p-3"
    >
      <IconLoader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
      <p className="text-sm text-muted-foreground">
        Probing agent capabilities… this usually takes a few seconds.
      </p>
    </div>
  );
}

function noAuthHint({ isAuth, canLogin }: { isAuth: boolean; canLogin: boolean }): React.ReactNode {
  if (!isAuth) {
    return "Install the agent CLI, then click Refresh.";
  }
  if (canLogin) {
    return "Click Open terminal to sign in, then refresh.";
  }
  return "Click Open terminal to launch a shell, run the agent's sign-in command, then refresh.";
}

function NoAuthActions({
  showTerminal,
  onOpenTerminal,
  onRefresh,
  isLoading,
}: {
  showTerminal: boolean;
  onOpenTerminal: () => void;
  onRefresh: () => Promise<void>;
  isLoading: boolean;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2">
      {showTerminal && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onOpenTerminal}
          className="cursor-pointer"
          data-testid="profile-no-auth-open-terminal"
        >
          <IconTerminal2 className="mr-2 h-4 w-4" />
          Open terminal
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={isLoading}
        className="cursor-pointer"
        data-testid="profile-no-auth-refresh"
      >
        <IconRefresh className={`mr-2 h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
        Refresh
      </Button>
    </div>
  );
}

export function NoAuthPanel({
  agentName,
  status,
  isLoading,
  onRefresh,
  error,
  rawError,
}: {
  agentName: string;
  status: "auth_required" | "not_installed";
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  rawError: string | null;
}) {
  const isAuth = status === "auth_required";
  const Icon = isAuth ? IconLock : IconPackageOff;
  const title = isAuth ? "No auth - login required" : "Not installed";
  const detail = error || rawError;
  const [loginOpen, setLoginOpen] = useState(false);
  const [shellOpen, setShellOpen] = useState(false);

  // Drives the "Open terminal" button. When the agent registers a
  // LoginCommand we open the dedicated login dialog (pre-fills the command);
  // otherwise we drop the user into a plain host shell so they can explore
  // (e.g. `<agent> --help`) and run whatever sign-in flow the CLI documents.
  const loginCommand = useAppStore((state) =>
    state.availableAgents.items.find((a) => a.name === agentName),
  )?.login_command;
  const canLogin = isAuth && Boolean(loginCommand);
  const showTerminal = isAuth;
  const hint = noAuthHint({ isAuth, canLogin });
  const handleOpenTerminal = () => {
    if (canLogin) setLoginOpen(true);
    else setShellOpen(true);
  };

  return (
    <div
      data-testid="profile-no-auth-panel"
      data-status={status}
      className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3"
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
                  className="inline-flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-foreground hover:text-foreground cursor-help"
                  data-testid="profile-no-auth-details"
                >
                  <IconAlertCircle className="h-3 w-3" />
                  details
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
        showTerminal={showTerminal}
        onOpenTerminal={handleOpenTerminal}
        onRefresh={onRefresh}
        isLoading={isLoading}
      />
      {canLogin && (
        <AgentLoginDialog
          open={loginOpen}
          onOpenChange={setLoginOpen}
          agentName={agentName}
          description={loginCommand?.description}
          command={loginCommand?.cmd}
          onLoginSuccess={() => {
            void onRefresh();
          }}
        />
      )}
      {showTerminal && !canLogin && (
        <HostShellDialog
          open={shellOpen}
          onOpenChange={setShellOpen}
          onClose={() => {
            void onRefresh();
          }}
        />
      )}
    </div>
  );
}
