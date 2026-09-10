"use client";

import {
  IconAlertCircle,
  IconCheck,
  IconCircleCheck,
  IconLoader2,
  IconMessageQuestion,
  IconPlayerPause,
  IconShieldQuestion,
  IconX,
} from "@tabler/icons-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { GridSpinner } from "@/components/grid-spinner";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { MobilePillButton } from "@/components/task/mobile/mobile-pill-button";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { isSessionActive, sortSessions } from "@/components/task/session-sort";
import { resolveThreadSessionStatus, type ThreadStatus } from "@/lib/threads/thread-session-status";

export type ThreadSessionView = {
  session: TaskSession;
  label: string;
  agentName: string | null;
  isPrimary: boolean;
};

const LABEL_SEPARATOR = " \u2022 ";

function sessionLabel(
  session: TaskSession,
  profilesById: Record<string, AgentProfileOption>,
  position: number,
  fallbackLabel: string,
): string {
  const profileLabel = session.agent_profile_id
    ? profilesById[session.agent_profile_id]?.label
    : undefined;
  const snapshotLabel =
    typeof session.agent_profile_snapshot?.label === "string"
      ? session.agent_profile_snapshot.label
      : undefined;
  const fullLabel = profileLabel ?? snapshotLabel;
  if (fullLabel) {
    const parts = fullLabel.split(LABEL_SEPARATOR);
    return parts[1] ?? parts[0] ?? fullLabel;
  }
  if (session.name) return session.name;
  return fallbackLabel ? fallbackLabel.replace("{{position}}", String(position)) : session.id;
}

export function buildThreadSessionViews(
  sessions: readonly TaskSession[],
  agentProfiles: readonly AgentProfileOption[] = [],
  fallbackLabel = "",
): ThreadSessionView[] {
  const profilesById = Object.fromEntries(agentProfiles.map((profile) => [profile.id, profile]));
  return sortSessions(sessions).map((session, index) => {
    const profile = session.agent_profile_id ? profilesById[session.agent_profile_id] : undefined;
    const snapshotAgentName = session.agent_profile_snapshot?.agent_id;
    return {
      session,
      label: sessionLabel(session, profilesById, index + 1, fallbackLabel),
      agentName:
        profile?.agent_name ??
        (typeof snapshotAgentName === "string" && snapshotAgentName ? snapshotAgentName : null),
      isPrimary: session.is_primary === true,
    };
  });
}

export function ThreadSessionStatusIcon({
  status,
  label,
  testId,
}: {
  status: ThreadStatus;
  label: string;
  testId?: string;
}) {
  const iconProps = {
    "aria-label": label,
    "data-testid": testId,
    className: "h-3.5 w-3.5 shrink-0",
  };
  switch (status.kind) {
    case "needs-you":
      return (
        <IconMessageQuestion {...iconProps} className={`${iconProps.className} text-yellow-500`} />
      );
    case "permission":
      return (
        <IconShieldQuestion {...iconProps} className={`${iconProps.className} text-amber-500`} />
      );
    case "clarification":
      return (
        <IconMessageQuestion {...iconProps} className={`${iconProps.className} text-yellow-500`} />
      );
    case "starting":
    case "working":
      return (
        <IconLoader2
          {...iconProps}
          className={`${iconProps.className} animate-spin text-blue-500`}
        />
      );
    case "failed":
      return <IconAlertCircle {...iconProps} className={`${iconProps.className} text-red-500`} />;
    case "cancelled":
      return <IconX {...iconProps} className={`${iconProps.className} text-muted-foreground`} />;
    case "finished":
      return (
        <IconCheck {...iconProps} className={`${iconProps.className} text-muted-foreground`} />
      );
    case "review-ready":
      return <IconCheck {...iconProps} className={`${iconProps.className} text-green-500`} />;
    case "completed":
      return <IconCircleCheck {...iconProps} className={`${iconProps.className} text-green-500`} />;
    case "waiting":
      return (
        <IconPlayerPause
          {...iconProps}
          className={`${iconProps.className} text-muted-foreground`}
        />
      );
    case "created":
      return (
        <IconAlertCircle
          {...iconProps}
          className={`${iconProps.className} text-muted-foreground`}
        />
      );
    default:
      return null;
  }
}

function DesktopSessionTabs({
  views,
  selectedSessionId,
  onSelect,
}: {
  views: ThreadSessionView[];
  selectedSessionId: string | null;
  onSelect: (sessionId: string) => void;
}) {
  const tabs: SessionTab[] = views.map((view) => ({
    id: view.session.id,
    label: view.label,
    icon: <ThreadSessionTabIndicator view={view} />,
    testId: `thread-session-tab-${view.session.id}`,
    className: "bg-transparent data-[state=active]:bg-muted",
  }));
  return (
    <div className="min-w-0 w-full" data-testid="thread-session-tabs">
      <SessionTabs
        tabs={tabs}
        activeTab={selectedSessionId ?? views[0]?.session.id ?? ""}
        onTabChange={onSelect}
        className="min-w-0 w-full"
        listClassName="min-w-0 w-full max-w-full shrink overflow-x-auto overflow-y-hidden bg-transparent p-0 !h-7 gap-1 [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]"
      />
    </div>
  );
}

function ThreadSessionTabIndicator({ view }: { view: ThreadSessionView }) {
  const { t } = useTranslation();
  const status = resolveThreadSessionStatus(view.session);
  if (status.kind === "permission" || status.kind === "clarification") {
    return (
      <ThreadSessionStatusIcon
        status={status}
        label={t(status.labelKey)}
        testId={`thread-session-status-${view.session.id}`}
      />
    );
  }
  return (
    <ThreadSessionAgentIndicator
      view={view}
      testId={`thread-session-agent-icon-${view.session.id}`}
    />
  );
}

function ThreadSessionAgentIndicator({
  view,
  testId,
}: {
  view: ThreadSessionView;
  testId: string;
}) {
  if (isSessionActive(view.session.state)) {
    return <GridSpinner className="h-3 w-3 shrink-0 text-muted-foreground" />;
  }
  if (view.agentName) {
    return (
      <span data-testid={testId} className="flex h-3 w-3 shrink-0 items-center" aria-hidden="true">
        <AgentLogo agentName={view.agentName} size={12} className="shrink-0" />
      </span>
    );
  }
  return (
    <span
      data-testid={testId}
      aria-hidden="true"
      className="h-3 w-3 shrink-0 rounded-full bg-muted-foreground/40"
    />
  );
}

function MobileSessionPicker({
  views,
  selectedSessionId,
  onSelect,
}: {
  views: ThreadSessionView[];
  selectedSessionId: string | null;
  onSelect: (sessionId: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selectedIndex = Math.max(
    0,
    views.findIndex((view) => view.session.id === selectedSessionId),
  );
  const selected = views[selectedIndex] ?? views[0];
  const handleSelect = (sessionId: string) => {
    onSelect(sessionId);
    setOpen(false);
  };

  return (
    <>
      <MobilePillButton
        label={selected?.label ?? t("threads:selectSession")}
        count={`${selectedIndex + 1}/${views.length}`}
        compact={false}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="thread-session-picker-trigger"
        ariaLabel={t("threads:selectSession")}
      />
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t("threads:selectSession")}
        description={t("threads:selectSessionDescription")}
        contentTestId="thread-session-picker-sheet"
      >
        <div role="list" className="flex flex-col gap-1">
          {views.map((view) => {
            const isSelected = view.session.id === selected?.session.id;
            return (
              <button
                key={view.session.id}
                type="button"
                aria-current={isSelected ? "true" : undefined}
                className="flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md border border-transparent px-3 py-2 text-left hover:bg-muted data-[selected=true]:border-primary/50 data-[selected=true]:bg-card"
                data-selected={isSelected ? "true" : undefined}
                data-testid={`thread-session-row-${view.session.id}`}
                onClick={() => handleSelect(view.session.id)}
              >
                <ThreadSessionTabIndicator view={view} />
                <span className="min-w-0 flex-1 truncate text-sm">{view.label}</span>
                {view.isPrimary && (
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {t("threads:primarySession")}
                  </span>
                )}
                {isSelected && <IconCheck aria-hidden="true" className="h-4 w-4 shrink-0" />}
              </button>
            );
          })}
        </div>
      </MobilePickerSheet>
    </>
  );
}

/**
 * Switch-only presentation for the existing sessions of one task. The
 * component owns no session lifecycle action and never changes task-page
 * active-session state.
 */
export function ThreadSessionSwitcher({
  sessions,
  selectedSessionId,
  onSelect,
}: {
  sessions: readonly TaskSession[];
  selectedSessionId: string | null;
  onSelect: (sessionId: string) => void;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const views = buildThreadSessionViews(sessions, agentProfiles);
  if (views.length <= 1) return null;

  return isMobile ? (
    <div className="min-w-0 shrink-0" data-testid="thread-session-switcher">
      <MobileSessionPicker
        views={views}
        selectedSessionId={selectedSessionId}
        onSelect={onSelect}
      />
    </div>
  ) : (
    <div className="min-w-0 max-w-[52%] shrink" data-testid="thread-session-switcher">
      <DesktopSessionTabs views={views} selectedSessionId={selectedSessionId} onSelect={onSelect} />
    </div>
  );
}
