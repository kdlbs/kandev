"use client";

import { useState, useCallback, useEffect, useRef, memo } from "react";
import {
  IconAlertTriangle,
  IconRefresh,
  IconPlayerPlay,
  IconTerminal2,
  IconCopy,
  IconCheck,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStoreApi } from "@/components/state-provider";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RecoveryAuthMethod, RecoveryMetadata } from "@/components/task/chat/types";

type RecoveryState = "pending" | "recovering" | "recovered" | "error";

export const AgentErrorRecoveryMessage = memo(function AgentErrorRecoveryMessage({
  comment,
  sessionState,
}: {
  comment: Message;
  sessionState?: TaskSessionState;
}) {
  const [state, setState] = useState<RecoveryState>("pending");
  const resetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const metadata = comment.metadata as RecoveryMetadata | undefined;
  const canRecover = Boolean(metadata?.task_id && metadata?.session_id);
  const storeApi = useAppStoreApi();

  useEffect(() => {
    return () => {
      if (resetTimerRef.current) clearTimeout(resetTimerRef.current);
    };
  }, []);

  const openBottomTerminal = useCallback(() => {
    const store = storeApi.getState();
    if (!store.bottomTerminal.isOpen) {
      store.toggleBottomTerminal();
    }
  }, [storeApi]);

  const openTerminalWithCommand = useCallback(
    (command: string) => {
      storeApi.getState().openBottomTerminalWithCommand(command);
    },
    [storeApi],
  );

  const handleRecover = useCallback(
    async (action: "resume" | "fresh_start") => {
      if (!canRecover || !metadata) return;
      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current);
        resetTimerRef.current = null;
      }
      const client = getWebSocketClient();
      if (!client) return;
      setState("recovering");
      try {
        await client.request("session.recover", {
          task_id: metadata.task_id,
          session_id: metadata.session_id,
          action,
        });
        setState("recovered");
      } catch (error) {
        console.error("Failed to recover session:", error);
        setState("error");
        resetTimerRef.current = setTimeout(() => {
          setState("pending");
          resetTimerRef.current = null;
        }, 3000);
      }
    },
    [canRecover, metadata],
  );

  // Hide once recovery succeeded or session is active again (handles page refresh)
  const isSessionActive =
    sessionState === "RUNNING" || sessionState === "STARTING" || sessionState === "COMPLETED";
  if (state === "recovered" || isSessionActive) {
    return null;
  }

  const isAuthError = metadata?.is_auth_error === true;
  const authMethods = metadata?.auth_methods;
  const message = comment.content || "Agent encountered an error";

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full rounded px-2 py-1 -mx-2">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle className="h-4 w-4 text-red-500" />
        </div>
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="text-xs font-mono text-red-600 dark:text-red-400">{message}</div>
          <ErrorActions
            isAuthError={isAuthError}
            authMethods={authMethods}
            openTerminalWithCommand={openTerminalWithCommand}
            openBottomTerminal={openBottomTerminal}
            state={state}
            canRecover={canRecover}
            hasResumeToken={metadata?.has_resume_token}
            onRecover={handleRecover}
          />
        </div>
      </div>
    </div>
  );
});

function ErrorActions({
  isAuthError,
  authMethods,
  openTerminalWithCommand,
  openBottomTerminal,
  state,
  canRecover,
  hasResumeToken,
  onRecover,
}: {
  isAuthError: boolean;
  authMethods: RecoveryAuthMethod[] | undefined;
  openTerminalWithCommand: (command: string) => void;
  openBottomTerminal: () => void;
  state: RecoveryState;
  canRecover: boolean;
  hasResumeToken?: boolean;
  onRecover: (action: "resume" | "fresh_start") => void;
}) {
  if (isAuthError) {
    const disabled = state === "recovering" || !canRecover;
    return (
      <>
        {authMethods && authMethods.length > 0 ? (
          <AuthMethodsPanel methods={authMethods} onOpenTerminal={openTerminalWithCommand} />
        ) : (
          <GenericAuthPanel onOpenTerminal={openBottomTerminal} />
        )}
        <div className="mt-2 flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-xs cursor-pointer gap-1.5"
                disabled={disabled}
                onClick={() => onRecover("fresh_start")}
                data-testid="recovery-restart-button"
              >
                <IconRefresh className="h-3 w-3" />
                Restart session
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">Restart the agent session after logging in</TooltipContent>
          </Tooltip>
          {state === "error" && <span className="text-xs text-red-500">Failed — try again</span>}
        </div>
      </>
    );
  }
  return (
    <RecoveryActions
      state={state}
      canRecover={canRecover}
      hasResumeToken={hasResumeToken}
      onRecover={onRecover}
    />
  );
}

function AuthMethodsPanel({
  methods,
  onOpenTerminal,
}: {
  methods: RecoveryAuthMethod[];
  onOpenTerminal: (command: string) => void;
}) {
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        Authentication required, log in before resuming
      </div>
      {methods.map((method) => (
        <AuthMethodRow key={method.id} method={method} onOpenTerminal={onOpenTerminal} />
      ))}
    </div>
  );
}

function RecoveryActions({
  state,
  canRecover,
  hasResumeToken,
  onRecover,
}: {
  state: RecoveryState;
  canRecover: boolean;
  hasResumeToken?: boolean;
  onRecover: (action: "resume" | "fresh_start") => void;
}) {
  const disabled = state === "recovering" || !canRecover;

  return (
    <div className="mt-2 flex items-center gap-2">
      {hasResumeToken && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className={cn("h-7 text-xs cursor-pointer gap-1.5")}
              disabled={disabled}
              onClick={() => onRecover("resume")}
              data-testid="recovery-resume-button"
            >
              <IconRefresh className="h-3 w-3" />
              Resume session
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top">
            Re-launch with resume flag — keeps all previous messages and context
          </TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className={cn("h-7 text-xs cursor-pointer gap-1.5")}
            disabled={disabled}
            onClick={() => onRecover("fresh_start")}
            data-testid="recovery-fresh-button"
          >
            <IconPlayerPlay className="h-3 w-3" />
            Start fresh session
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">
          New agent process on the same workspace — no previous conversation context
        </TooltipContent>
      </Tooltip>
      {state === "error" && <span className="text-xs text-red-500">Failed — try again</span>}
    </div>
  );
}

function GenericAuthPanel({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        Authentication required — please log in via the terminal
      </div>
      <Button
        variant="outline"
        size="sm"
        className="h-6 text-[11px] cursor-pointer gap-1 px-2"
        onClick={onOpenTerminal}
      >
        <IconTerminal2 className="h-3 w-3" />
        Open terminal
      </Button>
    </div>
  );
}

function buildFullCommand(termAuth: RecoveryAuthMethod["terminal_auth"]): string | null {
  if (!termAuth) return null;
  if (termAuth.args && termAuth.args.length > 0) {
    return `${termAuth.command} ${termAuth.args.join(" ")}`;
  }
  return termAuth.command;
}

function AuthMethodRow({
  method,
  onOpenTerminal,
}: {
  method: RecoveryAuthMethod;
  onOpenTerminal: (command: string) => void;
}) {
  const [copied, setCopied] = useState(false);
  const termAuth = method.terminal_auth;

  const fullCommand = buildFullCommand(termAuth);

  const handleCopy = useCallback(async () => {
    if (!fullCommand) return;
    try {
      await navigator.clipboard.writeText(fullCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard API unavailable
    }
  }, [fullCommand]);

  return (
    <div className="space-y-1">
      <div className="text-xs text-muted-foreground">{method.name}</div>
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
                className="h-6 w-6 p-0 cursor-pointer shrink-0"
                onClick={handleCopy}
                aria-label={copied ? "Command copied" : "Copy command"}
              >
                {copied ? (
                  <IconCheck className="h-3 w-3 text-green-500" />
                ) : (
                  <IconCopy className="h-3 w-3 text-muted-foreground" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">Copy command</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                className="h-6 text-[11px] cursor-pointer gap-1 px-2 shrink-0"
                onClick={() => fullCommand && onOpenTerminal(fullCommand)}
              >
                <IconTerminal2 className="h-3 w-3" />
                Run in terminal
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">
              Open the bottom terminal and paste this command
            </TooltipContent>
          </Tooltip>
        </div>
      ) : (
        <div className="text-xs text-muted-foreground">
          {method.description || "Login required"}
        </div>
      )}
    </div>
  );
}
