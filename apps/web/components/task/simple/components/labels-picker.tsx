"use client";

import { useRef, useState } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { addLabel, removeLabel } from "@/lib/api/domains/office-extended-api";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import type { Task, TaskLabelLocal } from "@/app/office/tasks/[id]/types";

type LabelsPickerProps = {
  task: Task;
};

function LabelChips({
  labels,
  onRemove,
}: {
  labels: TaskLabelLocal[];
  onRemove: (name: string) => void;
}) {
  return (
    <>
      {labels.map((label) => (
        <span
          key={label.name}
          className="inline-flex items-center gap-0.5 rounded-full px-2 py-0.5 text-xs font-medium"
          style={{ backgroundColor: `${label.color}20`, color: label.color }}
        >
          {label.name}
          <span
            role="button"
            tabIndex={0}
            className="ml-0.5 cursor-pointer opacity-60 hover:opacity-100 inline-flex"
            onClick={(e) => {
              e.stopPropagation();
              onRemove(label.name);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                e.stopPropagation();
                onRemove(label.name);
              }
            }}
            aria-label={`Remove ${label.name}`}
          >
            <IconX className="h-2.5 w-2.5" />
          </span>
        </span>
      ))}
    </>
  );
}

export function LabelsPicker({ task }: LabelsPickerProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [open, setOpen] = useState(false);
  const [newLabel, setNewLabel] = useState("");
  const [newColor, setNewColor] = useState("#6366f1");
  const inputRef = useRef<HTMLInputElement>(null);
  const mutate = useOptimisticTaskMutation();

  const labels = task.labels;

  const handleAdd = async () => {
    const name = newLabel.trim();
    if (!name || !workspaceId) return;
    if (labels.some((l) => l.name === name)) {
      toast.error("Label already exists");
      return;
    }
    const next = [...labels, { name, color: newColor }];
    setNewLabel("");
    try {
      await mutate(task.id, { labels: next }, () =>
        addLabel(workspaceId, task.id, { name, color: newColor }),
      );
    } catch {
      /* toast already raised */
    }
  };

  const handleRemove = async (name: string) => {
    if (!workspaceId) return;
    const next = labels.filter((l) => l.name !== name);
    try {
      await mutate(task.id, { labels: next }, () => removeLabel(workspaceId, task.id, name));
    } catch {
      /* toast already raised */
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          data-testid="labels-picker-trigger"
          className="flex flex-wrap items-center justify-end gap-1 ml-auto cursor-pointer rounded px-2 py-1 hover:bg-accent/50 min-h-[28px]"
        >
          {labels.length === 0 ? (
            <span className="text-muted-foreground text-xs">+ Add labels</span>
          ) : (
            <LabelChips labels={labels} onRemove={handleRemove} />
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-3 space-y-2" portal={false}>
        <div className="flex flex-wrap gap-1">
          {labels.length === 0 ? (
            <span className="text-xs text-muted-foreground">No labels yet</span>
          ) : (
            <LabelChips labels={labels} onRemove={handleRemove} />
          )}
        </div>
        <div className="flex items-center gap-1">
          <input
            ref={inputRef}
            className="flex-1 px-1.5 py-0.5 text-xs border border-border rounded bg-background"
            placeholder="label name"
            value={newLabel}
            onChange={(e) => setNewLabel(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                void handleAdd();
              }
              if (e.key === "Escape") setOpen(false);
            }}
            data-testid="labels-picker-input"
          />
          <input
            type="color"
            className="h-6 w-6 cursor-pointer rounded border-0 bg-transparent p-0"
            value={newColor}
            onChange={(e) => setNewColor(e.target.value)}
            aria-label="Label color"
          />
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer"
            onClick={() => void handleAdd()}
            aria-label="Add label"
          >
            <IconPlus className="h-3 w-3" />
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
