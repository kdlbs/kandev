"use client";

import { memo, useCallback, useMemo, useRef, useState } from "react";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { setSessionMode } from "@/lib/api/domains/session-api";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";

type ModeOption = {
  id: string;
  name: string;
  description?: string;
};

type ModeSelectorProps = {
  sessionId: string | null;
};

function resolveSnapshotMode(snapshot: unknown): string | null {
  if (!snapshot || typeof snapshot !== "object") return null;
  const mode = (snapshot as Record<string, unknown>).mode;
  return typeof mode === "string" && mode ? mode : null;
}

function resolveProfileMode(profileId: string | null | undefined, agents: Agent[]): string | null {
  if (!profileId) return null;
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (profile?.mode) return profile.mode;
  }
  return null;
}

function resolveStaticModes(
  agents: Agent[],
  profileId: string | null | undefined,
  availableAgents: AvailableAgent[],
): ModeOption[] {
  if (!profileId) return [];
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (!profile) continue;
    const available = availableAgents.find((a: AvailableAgent) => a.name === agent.name);
    return available?.model_config?.available_modes ?? [];
  }
  return [];
}

function formatModeName(modeId: string): string {
  return modeId
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function buildModeState(
  currentModeId: string | null,
  liveModes: ModeOption[] | undefined,
  staticModes: ModeOption[],
) {
  const availableModes = liveModes?.length ? liveModes : staticModes;
  if (!currentModeId) return undefined;
  if (availableModes.length === 0) {
    if (currentModeId === "default") return undefined;
    return {
      currentModeId,
      availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }],
    };
  }
  if (availableModes.length <= 1) return undefined;
  if (availableModes.some((m) => m.id === currentModeId)) {
    return { currentModeId, availableModes };
  }
  return {
    currentModeId,
    availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }, ...availableModes],
  };
}

function useModeSelectorState(sessionId: string | null) {
  useSettingsData(true);

  const liveModeState = useAppStore((state) =>
    sessionId ? state.sessionMode.bySessionId[sessionId] : undefined,
  );
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const { items: availableAgents } = useAvailableAgents();

  const session = sessionId ? (taskSessions[sessionId] ?? null) : null;
  const snapshotMode = resolveSnapshotMode(session?.agent_profile_snapshot);
  const profileMode = useMemo(
    () => resolveProfileMode(session?.agent_profile_id, settingsAgents as Agent[]),
    [session?.agent_profile_id, settingsAgents],
  );
  const staticModes = useMemo(
    () => resolveStaticModes(settingsAgents as Agent[], session?.agent_profile_id, availableAgents),
    [availableAgents, session?.agent_profile_id, settingsAgents],
  );

  return useMemo(
    () =>
      buildModeState(
        liveModeState?.currentModeId || snapshotMode || profileMode,
        liveModeState?.availableModes,
        staticModes,
      ),
    [liveModeState, profileMode, snapshotMode, staticModes],
  );
}

export const ModeSelector = memo(function ModeSelector({ sessionId }: ModeSelectorProps) {
  const modeState = useModeSelectorState(sessionId);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);

  const handleModeChange = useCallback(
    async (modeId: string) => {
      if (!sessionId) return;
      try {
        await setSessionMode(sessionId, modeId);
      } catch (err) {
        console.error("[ModeSelector] set-mode API failed:", err);
      }
    },
    [sessionId],
  );

  const handleDropdownOpenChange = useCallback((open: boolean) => {
    setDropdownOpen(open);
    if (!open) {
      recentlyClosedRef.current = true;
      setTooltipOpen(false);
      setTimeout(() => {
        recentlyClosedRef.current = false;
      }, 200);
    }
  }, []);

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (dropdownOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [dropdownOpen],
  );

  if (!sessionId || !modeState) {
    return null;
  }

  const currentMode = modeState.availableModes.find((m) => m.id === modeState.currentModeId);
  const displayName = currentMode?.name || modeState.currentModeId || "Mode";

  return (
    <DropdownMenu open={dropdownOpen} onOpenChange={handleDropdownOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={handleTooltipOpenChange}>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              data-testid="session-mode-selector"
              className="h-7 gap-1 px-2 cursor-pointer hover:bg-muted/40 whitespace-nowrap"
            >
              <span className="text-xs">{displayName}</span>
              <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="top">Agent permission mode</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="start" side="top" className="min-w-[280px]">
        <DropdownMenuLabel>Available Modes</DropdownMenuLabel>
        {modeState.availableModes.map((mode) => (
          <DropdownMenuItem
            key={mode.id}
            onClick={() => handleModeChange(mode.id)}
            className={`cursor-pointer relative pr-7 ${mode.id === modeState.currentModeId ? "bg-muted" : ""}`}
          >
            <div className="min-w-0 flex-1">
              <div>{mode.name}</div>
              {mode.description && (
                <div className="text-xs text-muted-foreground">{mode.description}</div>
              )}
            </div>
            {mode.id === modeState.currentModeId && (
              <IconCheck className="absolute right-2 h-4 w-4" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
});
