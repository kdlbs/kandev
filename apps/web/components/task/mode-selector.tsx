"use client";

import {
  forwardRef,
  memo,
  type ComponentPropsWithoutRef,
  useCallback,
  useMemo,
  useRef,
  useState,
} from "react";
import { IconAlertTriangle, IconCheck, IconChevronDown, IconX } from "@tabler/icons-react";
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
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { MobilePillButton } from "@/components/task/mobile/mobile-pill-button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { setSessionMode } from "@/lib/api/domains/session-api";
import { cn } from "@/lib/utils";
import { prioritizeSelectedOption } from "@/lib/utils/selector-options";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type ModeOption = {
  id: string;
  name: string;
  description?: string;
};

type ModeSelectorState = {
  currentModeId: string;
  availableModes: ModeOption[];
  requestedModeId?: string;
};

type ModeSelectorProps = {
  sessionId: string | null;
  triggerClassName?: string;
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

// withRequestedMode carries the requested mode onto the resolved state only
// when it differs from what the session is actually in.
function withRequestedMode(
  state: ModeSelectorState | undefined,
  requestedModeId: string | undefined,
): ModeSelectorState | undefined {
  if (!state || !requestedModeId || requestedModeId === state.currentModeId) return state;
  return { ...state, requestedModeId };
}

function buildModeState(
  currentModeId: string | null,
  liveModes: ModeOption[] | undefined,
  staticModes: ModeOption[],
): ModeSelectorState | undefined {
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
    return {
      currentModeId,
      availableModes: prioritizeSelectedOption(availableModes, currentModeId, (mode) => mode.id),
    };
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
      withRequestedMode(
        buildModeState(
          liveModeState?.currentModeId || snapshotMode || profileMode,
          liveModeState?.availableModes,
          staticModes,
        ),
        liveModeState?.requestedModeId,
      ),
    [liveModeState, profileMode, snapshotMode, staticModes],
  );
}

function ModeMismatchWarning({
  requestedName,
  displayName,
}: {
  requestedName: string;
  displayName: string;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="mx-4 mb-2 flex items-start gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-foreground"
      data-testid="session-mode-mismatch-warning"
      role="status"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden />
      <span>
        {t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })}
      </span>
    </div>
  );
}

function MobileModeOption({
  mode,
  selected,
  onSelect,
}: {
  mode: ModeOption;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      data-testid={`session-mode-option-${mode.id}`}
      aria-label={mode.name}
      aria-pressed={selected}
      className={cn(
        "relative flex min-h-11 w-full cursor-pointer items-center rounded-md border px-3 py-2 pr-10 text-left",
        selected ? "border-primary/50 bg-card font-medium" : "border-transparent hover:bg-muted/60",
      )}
      onClick={onSelect}
    >
      <span className="min-w-0 flex-1">
        <span className="block">{mode.name}</span>
        {mode.description && (
          <span className="block text-xs text-muted-foreground">{mode.description}</span>
        )}
      </span>
      {selected && <IconCheck className="absolute right-3 h-4 w-4" aria-hidden />}
    </button>
  );
}

function MobileModeOptions({
  modeState,
  onModeChange,
  onClose,
}: {
  modeState: ModeSelectorState;
  onModeChange: (modeId: string) => Promise<void>;
  onClose: () => void;
}) {
  return (
    <div className="space-y-1">
      {modeState.availableModes.map((mode) => (
        <MobileModeOption
          key={mode.id}
          mode={mode}
          selected={mode.id === modeState.currentModeId}
          onSelect={() => {
            void onModeChange(mode.id);
            onClose();
          }}
        />
      ))}
    </div>
  );
}

function MobileModeSelector({
  modeState,
  displayName,
  requestedName,
  triggerClassName,
  onModeChange,
}: {
  modeState: ModeSelectorState;
  displayName: string;
  requestedName?: string;
  triggerClassName?: string;
  onModeChange: (modeId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <MobilePillButton
        ref={triggerRef}
        icon={
          requestedName ? (
            <IconAlertTriangle className="h-3 w-3 shrink-0 text-amber-500" aria-hidden />
          ) : undefined
        }
        label={displayName}
        ariaLabel={displayName}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="session-mode-selector"
        className={cn("h-11 min-h-11 min-w-11", triggerClassName)}
      />
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t("task:agentPermissionMode")}
        description={t("task:availableModes")}
        contentTestId="session-mode-picker-scroll"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus({ preventScroll: true });
        }}
        fixedContent={
          requestedName ? (
            <ModeMismatchWarning requestedName={requestedName} displayName={displayName} />
          ) : undefined
        }
        headerAction={
          <Button
            variant="ghost"
            size="icon"
            className="h-11 w-11 shrink-0"
            aria-label={t("common:close")}
            data-testid="session-mode-mobile-picker-close"
            onClick={() => setOpen(false)}
          >
            <IconX className="h-4 w-4" aria-hidden />
          </Button>
        }
      >
        <MobileModeOptions
          modeState={modeState}
          onModeChange={onModeChange}
          onClose={() => setOpen(false)}
        />
      </MobilePickerSheet>
    </>
  );
}

// Rendered under DropdownMenuTrigger/TooltipTrigger `asChild`, so it must
// forward the ref and spread the props Radix attaches; swallowing them leaves
// the button inert.
const ModeSelectorTrigger = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<typeof Button> & {
    displayName: string;
    notApplied: boolean;
    triggerClassName?: string;
  }
>(function ModeSelectorTrigger({ displayName, notApplied, triggerClassName, ...props }, ref) {
  return (
    <Button
      {...props}
      ref={ref}
      variant="ghost"
      size="sm"
      data-testid="session-mode-selector"
      className={cn(
        "h-7 min-w-0 gap-1 overflow-hidden px-2 cursor-pointer whitespace-nowrap hover:bg-muted/40 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11",
        triggerClassName,
      )}
    >
      {notApplied && (
        <IconAlertTriangle
          className="h-3 w-3 text-amber-500 shrink-0"
          data-testid="session-mode-not-applied"
          aria-hidden
        />
      )}
      <span className="truncate text-xs">{displayName}</span>
      <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
    </Button>
  );
});

function useModeSelectorDropdownState() {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);

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

  return {
    dropdownOpen,
    tooltipOpen,
    handleDropdownOpenChange,
    handleTooltipOpenChange,
  };
}

export const ModeSelector = memo(function ModeSelector({
  sessionId,
  triggerClassName,
}: ModeSelectorProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const modeState = useModeSelectorState(sessionId);
  const { dropdownOpen, tooltipOpen, handleDropdownOpenChange, handleTooltipOpenChange } =
    useModeSelectorDropdownState();

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

  if (!sessionId || !modeState) {
    return null;
  }

  const currentMode = modeState.availableModes.find((m) => m.id === modeState.currentModeId);
  const displayName = currentMode?.name || modeState.currentModeId || t("common:mode");
  // Set only when the agent did not end up in the requested mode. Showing the
  // effective mode alone would be truthful but silent about the mismatch.
  const requestedName = modeState.requestedModeId
    ? (modeState.availableModes.find((m) => m.id === modeState.requestedModeId)?.name ??
      formatModeName(modeState.requestedModeId))
    : undefined;

  if (isMobile || !isFinePointer) {
    return (
      <MobileModeSelector
        modeState={modeState}
        displayName={displayName}
        requestedName={requestedName}
        triggerClassName={triggerClassName}
        onModeChange={handleModeChange}
      />
    );
  }

  return (
    <DropdownMenu open={dropdownOpen} onOpenChange={handleDropdownOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={handleTooltipOpenChange}>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <ModeSelectorTrigger
              displayName={displayName}
              notApplied={Boolean(requestedName)}
              triggerClassName={triggerClassName}
            />
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="top">
          {requestedName
            ? t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })
            : t("task:agentPermissionMode")}
        </TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="start" side="top" className="min-w-[280px]">
        <DropdownMenuLabel>{t("task:availableModes")}</DropdownMenuLabel>
        {modeState.availableModes.map((mode) => (
          <DropdownMenuItem
            key={mode.id}
            onClick={() => handleModeChange(mode.id)}
            className={cn(
              "relative min-h-11 cursor-pointer border border-transparent pr-7 sm:min-h-8",
              mode.id === modeState.currentModeId &&
                "border-primary/50 bg-card font-medium hover:bg-card",
            )}
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
