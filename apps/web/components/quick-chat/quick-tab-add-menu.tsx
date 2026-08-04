"use client";

import {
  IconCheck,
  IconMessageCircle,
  IconMessagePlus,
  IconPlus,
  IconSparkles,
  IconTerminal2,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import type {
  QuickChatActiveKind,
  QuickChatSession,
  QuickTerminalTab,
} from "@/lib/state/slices/ui/types";

type QuickTabAddMenuProps = {
  sessions: QuickChatSession[];
  terminalTabs: QuickTerminalTab[];
  activeKind: QuickChatActiveKind;
  activeSessionId: string | null;
  activeTerminalTabId: string | null;
  sessionLabel: (session: QuickChatSession, index: number) => string;
  onNewAgent: () => void;
  onActivateSession: (sessionId: string) => void;
  onNewTerminal: () => void;
  onActivateTerminal: (tabId: string) => void;
};

/** Grouped creation and activation menu for the shared Quick Chat surface. */
export function QuickTabAddMenu({
  sessions,
  terminalTabs,
  activeKind,
  activeSessionId,
  activeTerminalTabId,
  sessionLabel,
  onNewAgent,
  onActivateSession,
  onNewTerminal,
  onActivateTerminal,
}: QuickTabAddMenuProps) {
  const { t } = useTranslation();
  const sortedTerminalTabs = [...terminalTabs].sort((a, b) => a.sequence - b.sequence);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="h-11 w-11 shrink-0 cursor-pointer sm:h-6 sm:w-6"
          aria-label={t("sidebar:quickChatAdd")}
          data-testid="quick-chat-add-menu-trigger"
        >
          <IconPlus className="h-4 w-4 sm:h-3.5 sm:w-3.5" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          {t("common:agents")}
        </DropdownMenuLabel>
        <DropdownMenuItem
          onSelect={onNewAgent}
          className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
          data-testid="quick-chat-new-agent"
        >
          <IconMessagePlus className="h-3.5 w-3.5 shrink-0" aria-hidden />
          <span className="flex-1 truncate">{t("sidebar:quickChatNewAgent")}</span>
        </DropdownMenuItem>
        {sessions.map((session, index) => {
          const isActive = activeKind === "conversation" && session.sessionId === activeSessionId;
          return (
            <DropdownMenuItem
              key={session.sessionId}
              onSelect={() => onActivateSession(session.sessionId)}
              aria-current={isActive ? "page" : undefined}
              className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
              data-testid={`quick-chat-menu-session-${session.sessionId}`}
            >
              {session.kind === "config" ? (
                <IconSparkles className="h-3.5 w-3.5 shrink-0" aria-hidden />
              ) : (
                <IconMessageCircle className="h-3.5 w-3.5 shrink-0" aria-hidden />
              )}
              <span className="flex-1 truncate">{sessionLabel(session, index)}</span>
              {isActive && <IconCheck className="h-3.5 w-3.5 shrink-0" aria-hidden />}
            </DropdownMenuItem>
          );
        })}
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          {t("sidebar:quickChatTerminals")}
        </DropdownMenuLabel>
        <DropdownMenuItem
          onSelect={onNewTerminal}
          className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
          data-testid="quick-chat-new-terminal"
        >
          <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
          <span className="flex-1 truncate">{t("sidebar:quickChatNewTerminal")}</span>
        </DropdownMenuItem>
        {sortedTerminalTabs.map((tab) => {
          const label = t("sidebar:quickChatTerminalTab", { count: tab.sequence });
          const isActive = activeKind === "terminal" && tab.tabId === activeTerminalTabId;
          return (
            <DropdownMenuItem
              key={tab.tabId}
              onSelect={() => onActivateTerminal(tab.tabId)}
              aria-current={isActive ? "page" : undefined}
              title={tab.error ? t("sidebar:quickChatTerminalError", { error: tab.error }) : label}
              className="min-h-11 cursor-pointer gap-1.5 sm:min-h-7"
              data-testid={`quick-chat-menu-terminal-${tab.tabId}`}
            >
              <IconTerminal2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
              <span className="flex-1 truncate">{label}</span>
              {isActive && <IconCheck className="h-3.5 w-3.5 shrink-0" aria-hidden />}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
