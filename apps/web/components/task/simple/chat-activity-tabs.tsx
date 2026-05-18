"use client";

import { useMemo } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useAppStore } from "@/components/state-provider";
import { agentTint } from "@/app/office/components/agent-avatar";
import { TaskChat } from "./task-chat";
import { TaskActivity } from "./task-activity";
import { AdvancedChatPanel } from "@/app/office/tasks/[id]/advanced-panels/chat-panel";
import {
  groupSessionsForTimeline,
  isGroupLive,
  isOfficeGroup,
  type SessionGroup,
} from "./session-groups";
import { ApprovalActionBar } from "./components/approval-action-bar";
import type {
  Task,
  TaskActivityEntry,
  TaskComment,
  TaskSession,
  TimelineEvent,
} from "@/app/office/tasks/[id]/types";

// TAB_TRIGGER_BASE adds a 1px ring to the active tab on top of
// shadcn's default bg/text active styling, so the selected tab is
// unambiguous across Chat / Activity / agent triggers without
// depending on subtle bg contrast.
const TAB_TRIGGER_BASE =
  "cursor-pointer data-[state=active]:ring-1 data-[state=active]:ring-border";

function AgentTabTrigger({ group }: { group: SessionGroup }) {
  const live = isGroupLive(group);
  const agentProfileId = group.representative.agentProfileId;
  // Resolve the agent's display name + role from the office store so a
  // rename flows through automatically and we never fall back to the
  // UUID that lands in `session.agentName` when the session's profile
  // snapshot is empty.
  const resolved = useAppStore((s) =>
    agentProfileId ? s.office.agentProfiles.find((a) => a.id === agentProfileId) : undefined,
  );
  const label = resolved?.name || group.representative.agentName || "Agent";
  // Apply the per-agent tint only when the tab is active, so the
  // selected state reads clearly. Inactive tabs inherit shadcn's muted
  // styling — same as Chat / Activity — with a small dot in the agent
  // color hinting at identity.
  const activeTint = agentTint(label)
    .split(/\s+/)
    .filter(Boolean)
    .map((c) => `data-[state=active]:${c}`)
    .join(" ");
  return (
    <TabsTrigger
      value={`agent-${group.id}`}
      data-testid={`agent-tab-${group.id}`}
      className={`${TAB_TRIGGER_BASE} ${activeTint}`}
    >
      <span className="inline-flex items-center gap-1.5">
        <span className={`inline-block h-1.5 w-1.5 rounded-full ${agentDot(label)}`} aria-hidden />
        {label}
        {live && (
          <span
            data-testid="agent-tab-live-dot"
            className="inline-block h-1.5 w-1.5 rounded-full bg-primary animate-pulse"
            aria-label="agent running"
          />
        )}
      </span>
    </TabsTrigger>
  );
}

// agentDot returns just the background color class from the agent
// tint, so inactive tabs can show a tiny color dot for identification
// without bleeding the full tint into the tab background.
function agentDot(label: string): string {
  const bg = agentTint(label)
    .split(/\s+/)
    .find((c) => c.startsWith("bg-"));
  return bg ? bg.replace("/15", "") : "bg-muted-foreground/40";
}

function AgentTabContent({ taskId, group }: { taskId: string; group: SessionGroup }) {
  // Pick the active session if any; otherwise fall back to the
  // representative (most-recent). Mirrors `SessionTimelineEntry`.
  const active = group.group.find((s) => s.state === "RUNNING");
  const embedSessionId = active?.id ?? group.representative.id;
  return (
    <TabsContent value={`agent-${group.id}`}>
      <div
        data-testid={`agent-tab-embed-${group.id}`}
        className="h-[500px] flex flex-col overflow-hidden"
      >
        <AdvancedChatPanel taskId={taskId} sessionId={embedSessionId} hideInput />
      </div>
    </TabsContent>
  );
}

type ChatActivityTabsProps = {
  task: Task;
  comments: TaskComment[];
  timeline?: TimelineEvent[];
  activity: TaskActivityEntry[];
  sessions: TaskSession[];
  scrollParent: HTMLElement | null;
  readOnly: boolean;
  onCommentsChanged?: () => void;
};

/**
 * Tab strip below the task properties: `Chat | Activity | <agent-1> | <agent-2> | …`.
 *
 * Office task sessions (one per (task, agent)) become per-agent sibling
 * tabs whose content embeds the agent's session transcript via
 * `AdvancedChatPanel`. Kanban / quick-chat sessions still render inline
 * inside the Chat tab via `TaskChat`.
 */
export function ChatActivityTabs({
  task,
  comments,
  timeline,
  activity,
  sessions,
  scrollParent,
  readOnly,
  onCommentsChanged,
}: ChatActivityTabsProps) {
  const officeGroups = useMemo(
    () => groupSessionsForTimeline(sessions, task.reviewers, task.approvers).filter(isOfficeGroup),
    [sessions, task.reviewers, task.approvers],
  );

  return (
    <Tabs defaultValue="chat" className="mt-6">
      <TabsList>
        <TabsTrigger value="chat" className={TAB_TRIGGER_BASE}>
          Chat
        </TabsTrigger>
        <TabsTrigger value="activity" className={TAB_TRIGGER_BASE}>
          Activity
        </TabsTrigger>
        {officeGroups.map((g) => (
          <AgentTabTrigger key={g.id} group={g} />
        ))}
      </TabsList>
      <TabsContent value="chat">
        <ApprovalActionBar task={task} />
        <TaskChat
          taskId={task.id}
          comments={comments}
          timeline={timeline}
          sessions={sessions}
          decisions={task.decisions}
          reviewers={task.reviewers}
          approvers={task.approvers}
          scrollParent={scrollParent}
          readOnly={readOnly}
          onCommentsChanged={onCommentsChanged}
          taskTitle={task.title}
          taskDescription={task.description}
        />
      </TabsContent>
      <TabsContent value="activity">
        <TaskActivity taskId={task.id} entries={activity} />
      </TabsContent>
      {officeGroups.map((g) => (
        <AgentTabContent key={g.id} taskId={task.id} group={g} />
      ))}
    </Tabs>
  );
}
