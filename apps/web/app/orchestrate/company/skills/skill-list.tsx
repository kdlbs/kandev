"use client";

import { IconBoxMultiple, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { Skill } from "@/lib/state/slices/orchestrate/types";

type SkillListProps = {
  skills: Skill[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onAdd: () => void;
};

export function SkillList({ skills, selectedId, onSelect, onAdd }: SkillListProps) {
  return (
    <div className="w-[300px] border-r border-border overflow-y-auto p-4 shrink-0">
      <Button className="w-full mb-4 cursor-pointer" onClick={onAdd}>
        <IconPlus className="h-4 w-4 mr-1" /> Add Skill
      </Button>
      <div className="space-y-1">
        {skills.length === 0 && (
          <p className="text-sm text-muted-foreground px-3 py-2">No skills yet</p>
        )}
        {skills.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => onSelect(s.id)}
            className={cn(
              "flex items-center gap-2 px-3 py-2 rounded-md text-sm w-full text-left cursor-pointer",
              selectedId === s.id ? "bg-accent" : "hover:bg-accent/50",
            )}
          >
            <IconBoxMultiple className="h-4 w-4 text-muted-foreground shrink-0" />
            <span className="truncate flex-1">{s.name}</span>
            <Badge variant="outline" className="ml-auto text-[10px] shrink-0">
              {s.sourceType}
            </Badge>
          </button>
        ))}
      </div>
    </div>
  );
}
