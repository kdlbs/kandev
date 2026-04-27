"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { IconCode, IconCopy, IconPlus, IconUpload } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import { TaskProperties } from "./task-properties";
import { TaskChat } from "./task-chat";
import { TaskActivity } from "./task-activity";
import { SessionTabs } from "./session-tabs";
import { StatusIcon } from "./status-icon";
import { NewIssueDialog } from "../../components/new-issue-dialog";
import type { Issue, IssueComment, IssueActivityEntry, TaskSession } from "./types";

type TaskSimpleModeProps = {
  task: Issue;
  comments: IssueComment[];
  activity: IssueActivityEntry[];
  sessions: TaskSession[];
  onToggleAdvanced?: () => void;
};

function TaskBreadcrumb({ task }: { task: Issue }) {
  return (
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link href="/orchestrate/issues">Issues</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        {task.parentIdentifier && (
          <>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href={`/orchestrate/issues/${task.parentId}`}>{task.parentTitle}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
          </>
        )}
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbPage>{task.title}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}

function TaskHeaderRow({ task, onToggleAdvanced }: { task: Issue; onToggleAdvanced?: () => void }) {
  return (
    <div className="flex items-center gap-2 mt-4">
      <StatusIcon status={task.status} />
      <span className="text-sm font-mono text-muted-foreground">{task.identifier}</span>
      {task.projectName && <Badge variant="outline">{task.projectName}</Badge>}
      <div className="ml-auto flex gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              disabled={!onToggleAdvanced}
              onClick={onToggleAdvanced}
            >
              <IconCode className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {onToggleAdvanced
              ? "Advanced mode (terminal, files, plan)"
              : "No agent session available"}
          </TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={() => navigator.clipboard.writeText(task.identifier)}
            >
              <IconCopy className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Copy identifier</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

function ChildIssuesList({ items }: { items: Issue["children"] }) {
  if (items.length === 0) return null;
  return (
    <div className="mt-8">
      <h2 className="text-sm font-semibold mb-4">Sub-issues</h2>
      <div className="border border-border rounded-lg divide-y divide-border">
        {items.map((child) => (
          <Link
            key={child.id}
            href={`/orchestrate/issues/${child.id}`}
            className="flex items-center gap-2 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors"
          >
            <StatusIcon status={child.status} className="h-3.5 w-3.5 shrink-0" />
            <span className="text-xs text-muted-foreground font-mono shrink-0">
              {child.identifier}
            </span>
            <span className="flex-1 truncate">{child.title}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}

function ChatActivityTabs({
  task,
  comments,
  activity,
  sessions,
  activeSessionId,
  hasMultipleSessions,
  activeSession,
  onSelectSession,
}: {
  task: Issue;
  comments: IssueComment[];
  activity: IssueActivityEntry[];
  sessions: TaskSession[];
  activeSessionId: string;
  hasMultipleSessions: boolean;
  activeSession: TaskSession | undefined;
  onSelectSession: (id: string) => void;
}) {
  return (
    <Tabs defaultValue="chat" className="mt-6">
      <TabsList>
        <TabsTrigger value="chat" className="cursor-pointer">
          Chat
        </TabsTrigger>
        <TabsTrigger value="activity" className="cursor-pointer">
          Activity
        </TabsTrigger>
      </TabsList>
      <TabsContent value="chat">
        {hasMultipleSessions ? (
          <SessionTabs
            sessions={sessions}
            activeSessionId={activeSessionId}
            onSelect={onSelectSession}
          />
        ) : null}
        <TaskChat
          taskId={task.id}
          comments={comments}
          readOnly={activeSession?.state === "COMPLETED" || activeSession?.state === "FAILED"}
        />
      </TabsContent>
      <TabsContent value="activity">
        <TaskActivity taskId={task.id} entries={activity} />
      </TabsContent>
    </Tabs>
  );
}

export function TaskSimpleMode({
  task,
  comments,
  activity,
  sessions,
  onToggleAdvanced,
}: TaskSimpleModeProps) {
  const [subIssueOpen, setSubIssueOpen] = useState(false);
  const hasMultipleSessions = sessions.length >= 2;
  const [activeSessionId, setActiveSessionId] = useState<string>(sessions[0]?.id ?? "");
  const activeSession = useMemo(
    () => sessions.find((s) => s.id === activeSessionId),
    [sessions, activeSessionId],
  );

  return (
    <div className="flex h-full">
      <div className="flex-1 min-w-0 overflow-y-auto p-6">
        <TaskBreadcrumb task={task} />
        <TaskHeaderRow task={task} onToggleAdvanced={onToggleAdvanced} />
        <h1 className="text-xl font-semibold mt-4">{task.title}</h1>
        {task.description && (
          <div className="prose prose-sm mt-4 max-w-none text-sm whitespace-pre-wrap">
            {task.description}
          </div>
        )}
        <div className="flex gap-2 mt-6">
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => setSubIssueOpen(true)}
          >
            <IconPlus className="h-3.5 w-3.5 mr-1" /> New Sub-Issue
          </Button>
          <Button variant="outline" size="sm" className="cursor-pointer">
            <IconUpload className="h-3.5 w-3.5 mr-1" /> Upload attachment
          </Button>
          <Button variant="outline" size="sm" className="cursor-pointer">
            <IconPlus className="h-3.5 w-3.5 mr-1" /> New document
          </Button>
        </div>
        <ChatActivityTabs
          task={task}
          comments={comments}
          activity={activity}
          sessions={sessions}
          activeSessionId={activeSessionId}
          hasMultipleSessions={hasMultipleSessions}
          activeSession={activeSession}
          onSelectSession={setActiveSessionId}
        />
        <ChildIssuesList items={task.children} />
      </div>
      <div className="w-80 border-l border-border shrink-0 overflow-y-auto p-4">
        <TaskProperties task={task} />
      </div>
      <NewIssueDialog
        open={subIssueOpen}
        onOpenChange={setSubIssueOpen}
        parentTaskId={task.id}
        defaultProjectId={task.projectId}
        defaultAssigneeId={task.assigneeAgentInstanceId}
      />
    </div>
  );
}
