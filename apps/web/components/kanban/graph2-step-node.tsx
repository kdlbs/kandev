"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  IconCheck,
  IconCircleDashed,
  IconChevronLeft,
  IconChevronRight,
} from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { linkToSession } from "@/lib/links";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

type StepPhase = "past" | "current" | "future";

function isRunningState(state?: string): boolean {
  return state === "IN_PROGRESS" || state === "SCHEDULING";
}

export type Graph2StepNodeProps = {
  step: WorkflowStep;
  phase: StepPhase;
  task: Task;
  hasPrev: boolean;
  hasNext: boolean;
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  prevStepId?: string;
  nextStepId?: string;
  isMoving?: boolean;
};

const NODE_CLASS =
  "w-[130px] h-[36px] rounded-lg shrink-0 px-2.5 flex flex-col items-start justify-center";

function PastNode({ step }: { step: WorkflowStep }) {
  return (
    <div className={cn(NODE_CLASS, "border border-muted-foreground/20 bg-muted/30")}>
      <div className="flex items-center gap-1.5 w-full">
        <IconCheck className="h-3 w-3 text-green-500 shrink-0" />
        <span className="text-[11px] text-muted-foreground truncate">{step.title}</span>
      </div>
    </div>
  );
}

function FutureNode({ step }: { step: WorkflowStep }) {
  return (
    <div className={cn(NODE_CLASS, "border border-dashed border-muted-foreground/20 bg-muted/10")}>
      <div className="flex items-center gap-1.5 w-full">
        <IconCircleDashed className="h-3 w-3 text-muted-foreground/40 shrink-0" />
        <span className="text-[11px] text-muted-foreground/40 truncate">{step.title}</span>
      </div>
    </div>
  );
}

function MoveButton({
  direction,
  isMoving,
  onClick,
}: {
  direction: "left" | "right";
  isMoving?: boolean;
  onClick: (e: React.MouseEvent) => void;
}) {
  const posClass = direction === "left" ? "-left-3" : "-right-3";
  const Icon = direction === "left" ? IconChevronLeft : IconChevronRight;
  return (
    <button
      type="button"
      disabled={isMoving}
      onClick={onClick}
      className={cn(
        `absolute ${posClass} top-1/2 -translate-y-1/2 z-10`,
        "h-5 w-5 rounded-full bg-background border border-border shadow-sm",
        "flex items-center justify-center",
        "hover:bg-accent transition-colors cursor-pointer",
        isMoving && "opacity-50 cursor-not-allowed",
      )}
    >
      <Icon className="h-3 w-3" />
    </button>
  );
}

export function Graph2StepNode({
  step,
  phase,
  task,
  hasPrev,
  hasNext,
  onMoveTask,
  onPreviewTask,
  prevStepId,
  nextStepId,
  isMoving,
}: Graph2StepNodeProps) {
  const router = useRouter();
  const [isHovered, setIsHovered] = useState(false);

  if (phase === "past") return <PastNode step={step} />;
  if (phase === "future") return <FutureNode step={step} />;

  // Current phase
  const running = isRunningState(task.state);

  const handleClick = () => {
    if (task.primarySessionId) {
      router.push(linkToSession(task.primarySessionId));
    } else {
      onPreviewTask(task);
    }
  };

  return (
    <div
      className="relative shrink-0"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {isHovered && hasPrev && prevStepId && (
        <MoveButton
          direction="left"
          isMoving={isMoving}
          onClick={(e) => {
            e.stopPropagation();
            onMoveTask(task, prevStepId);
          }}
        />
      )}

      <button
        type="button"
        onClick={handleClick}
        className={cn(
          NODE_CLASS,
          "cursor-pointer transition-colors bg-background hover:bg-accent/30",
          running
            ? "border-1 border-accent/50 node-border-running"
            : "border-1 border-accent/50",
        )}
      >
        <div className="flex items-center gap-1.5 w-full">
          <div className="shrink-0">{getTaskStateIcon(task.state, "h-3 w-3")}</div>
          <span className="text-[11px] font-medium text-foreground truncate">
            {step.title}
          </span>
        </div>
      </button>

      {isHovered && hasNext && nextStepId && (
        <MoveButton
          direction="right"
          isMoving={isMoving}
          onClick={(e) => {
            e.stopPropagation();
            onMoveTask(task, nextStepId);
          }}
        />
      )}
    </div>
  );
}
