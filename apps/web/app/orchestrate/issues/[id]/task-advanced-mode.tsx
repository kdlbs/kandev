"use client";

import { useState } from "react";
import {
  IconArrowLeft,
  IconLayoutList,
  IconMessage,
  IconTerminal2,
  IconListDetails,
  IconFiles,
  IconGitBranch,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { StatusIcon } from "./status-icon";
import { AdvancedChatPanel } from "./advanced-panels/chat-panel";
import { AdvancedTerminalPanel } from "./advanced-panels/terminal-panel";
import { AdvancedPlanPanel } from "./advanced-panels/plan-panel";
import { AdvancedFilesPanel } from "./advanced-panels/files-panel";
import { AdvancedChangesPanel } from "./advanced-panels/changes-panel";
import type { Issue } from "./types";

type TaskAdvancedModeProps = {
  task: Issue;
  onToggleSimple: () => void;
};

export function TaskAdvancedMode({ task, onToggleSimple }: TaskAdvancedModeProps) {
  const [rightTab, setRightTab] = useState<"files" | "changes">("files");
  const [rightCollapsed, setRightCollapsed] = useState(false);

  return (
    <div className="flex flex-col h-full">
      {/* Topbar */}
      <div className="flex items-center gap-2 px-4 h-10 border-b border-border bg-background shrink-0">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={onToggleSimple}
            >
              <IconArrowLeft className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Back</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={onToggleSimple}
            >
              <IconLayoutList className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Switch to simple mode</TooltipContent>
        </Tooltip>
        <Separator orientation="vertical" className="h-5" />
        <StatusIcon status={task.status} className="h-4 w-4" />
        <span className="text-xs font-mono text-muted-foreground">{task.identifier}</span>
        <span className="text-sm font-medium truncate">{task.title}</span>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 cursor-pointer"
                onClick={() => setRightCollapsed(!rightCollapsed)}
              >
                <IconFiles className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{rightCollapsed ? "Show files panel" : "Hide files panel"}</TooltipContent>
          </Tooltip>
          {task.assigneeName && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <div className="h-6 w-6 rounded-full bg-muted flex items-center justify-center">
                <span className="text-[10px] font-medium">
                  {task.assigneeName.charAt(0).toUpperCase()}
                </span>
              </div>
              {task.assigneeName}
            </div>
          )}
        </div>
      </div>

      {/* Main content area */}
      <div className="flex-1 min-h-0 flex">
        {/* Left: Tabbed panels */}
        <div className="flex-1 min-w-0 flex flex-col">
          <Tabs defaultValue="chat" className="flex flex-col h-full">
            <div className="border-b border-border px-2 shrink-0">
              <TabsList className="h-9 bg-transparent p-0 gap-0">
                <TabsTrigger
                  value="chat"
                  className="cursor-pointer h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent gap-1.5 px-3"
                >
                  <IconMessage className="h-3.5 w-3.5" />
                  Chat
                </TabsTrigger>
                <TabsTrigger
                  value="terminal"
                  className="cursor-pointer h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent gap-1.5 px-3"
                >
                  <IconTerminal2 className="h-3.5 w-3.5" />
                  Terminal
                </TabsTrigger>
                <TabsTrigger
                  value="plan"
                  className="cursor-pointer h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent gap-1.5 px-3"
                >
                  <IconListDetails className="h-3.5 w-3.5" />
                  Plan
                </TabsTrigger>
              </TabsList>
            </div>
            <TabsContent value="chat" className="flex-1 min-h-0 mt-0">
              <AdvancedChatPanel taskId={task.id} />
            </TabsContent>
            <TabsContent value="terminal" className="flex-1 min-h-0 mt-0">
              <AdvancedTerminalPanel taskId={task.id} />
            </TabsContent>
            <TabsContent value="plan" className="flex-1 min-h-0 mt-0">
              <AdvancedPlanPanel taskId={task.id} />
            </TabsContent>
          </Tabs>
        </div>

        {/* Right: Files / Changes sidebar */}
        {!rightCollapsed && (
          <div className="w-72 xl:w-80 border-l border-border shrink-0 flex flex-col">
            <div className="border-b border-border px-2 shrink-0">
              <div className="flex h-9 gap-0">
                <button
                  className={`cursor-pointer flex items-center gap-1.5 px-3 h-9 text-xs font-medium border-b-2 transition-colors ${
                    rightTab === "files"
                      ? "border-primary text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                  onClick={() => setRightTab("files")}
                >
                  <IconFiles className="h-3.5 w-3.5" />
                  Files
                </button>
                <button
                  className={`cursor-pointer flex items-center gap-1.5 px-3 h-9 text-xs font-medium border-b-2 transition-colors ${
                    rightTab === "changes"
                      ? "border-primary text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                  onClick={() => setRightTab("changes")}
                >
                  <IconGitBranch className="h-3.5 w-3.5" />
                  Changes
                </button>
              </div>
            </div>
            <ScrollArea className="flex-1">
              {rightTab === "files" ? (
                <AdvancedFilesPanel taskId={task.id} />
              ) : (
                <AdvancedChangesPanel taskId={task.id} />
              )}
            </ScrollArea>
          </div>
        )}
      </div>
    </div>
  );
}
