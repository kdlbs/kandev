"use client";

import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";

type ParticipantRowProps = {
  label: string;
  agents: AgentInstance[];
  selectedIds: string[];
  onSelect: (ids: string[]) => void;
  onHide: () => void;
};

export function ParticipantRow({
  label,
  agents,
  selectedIds,
  onSelect,
  onHide,
}: ParticipantRowProps) {
  const toggle = (id: string) => {
    onSelect(selectedIds.includes(id) ? selectedIds.filter((x) => x !== id) : [...selectedIds, id]);
  };

  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      <span className="w-16 shrink-0">{label}</span>
      <Popover>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
            {selectedIds.length > 0
              ? `${selectedIds.length} selected`
              : `Add ${label.toLowerCase()}`}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-48 p-1" align="start">
          {agents.map((agent) => (
            <button
              key={agent.id}
              type="button"
              className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer flex items-center gap-2"
              onClick={() => toggle(agent.id)}
            >
              <span
                className={`h-3 w-3 rounded-sm border ${selectedIds.includes(agent.id) ? "bg-primary border-primary" : "border-muted-foreground"}`}
              />
              {agent.name}
            </button>
          ))}
        </PopoverContent>
      </Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className="h-6 w-6 cursor-pointer" onClick={onHide}>
            <IconX className="h-3 w-3" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Remove {label.toLowerCase()}</TooltipContent>
      </Tooltip>
    </div>
  );
}
