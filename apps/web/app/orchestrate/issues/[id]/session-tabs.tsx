"use client";

import { cn } from "@/lib/utils";
import type { TaskSession } from "./types";

type SessionTabsProps = {
  sessions: TaskSession[];
  activeSessionId: string;
  onSelect: (id: string) => void;
};

export function SessionTabs({ sessions, activeSessionId, onSelect }: SessionTabsProps) {
  return (
    <div className="flex gap-1 border-b border-border mb-3 -mt-1">
      {sessions.map((session) => {
        const isActive = session.id === activeSessionId;
        const isRunning = session.state === "RUNNING";
        return (
          <button
            key={session.id}
            onClick={() => onSelect(session.id)}
            className={cn(
              "cursor-pointer flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium border-b-2 transition-colors",
              isActive
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {session.agentName || "Agent"}
            {isRunning && <span className="h-2 w-2 rounded-full bg-cyan-400 animate-pulse" />}
          </button>
        );
      })}
    </div>
  );
}
