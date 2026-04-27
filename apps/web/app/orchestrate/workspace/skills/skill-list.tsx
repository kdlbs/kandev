"use client";

import { useState } from "react";
import {
  IconBoxMultiple,
  IconPlus,
  IconRefresh,
  IconExternalLink,
  IconBrandGithub,
  IconFolder,
  IconCode,
  IconLoader2,
} from "@tabler/icons-react";
import { toast } from "sonner";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { Skill, SkillSourceType } from "@/lib/state/slices/orchestrate/types";

interface SkillListProps {
  skills: Skill[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onRefresh: () => void;
  onImport: (source: string) => Promise<void>;
}

function sourceIcon(sourceType: SkillSourceType) {
  switch (sourceType) {
    case "git":
    case "skills_sh":
      return <IconBrandGithub className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
    case "local_path":
      return <IconFolder className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
    default:
      return <IconCode className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
  }
}

export function SkillList(props: SkillListProps) {
  const { skills, selectedId, onSelect, onAdd, onRefresh, onImport } = props;
  const [search, setSearch] = useState("");
  const [importSource, setImportSource] = useState("");
  const [importing, setImporting] = useState(false);

  const filtered = search
    ? skills.filter(
        (s) =>
          s.name.toLowerCase().includes(search.toLowerCase()) ||
          s.slug.toLowerCase().includes(search.toLowerCase()),
      )
    : skills;

  const handleImport = async () => {
    if (!importSource.trim() || importing) return;
    setImporting(true);
    try {
      await onImport(importSource.trim());
      setImportSource("");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Unknown error";
      toast.error(`Failed to import skill: ${msg}`);
    } finally {
      setImporting(false);
    }
  };

  return (
    <div className="w-[300px] border-r border-border overflow-y-auto shrink-0 flex flex-col">
      <SkillListHeader count={skills.length} onRefresh={onRefresh} onAdd={onAdd} />
      <SkillImportInput
        value={importSource}
        onChange={setImportSource}
        onImport={handleImport}
        importing={importing}
      />
      <div className="px-4 pb-2">
        <Input
          placeholder="Filter skills..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 text-xs"
        />
      </div>
      <SkillItems items={filtered} selectedId={selectedId} onSelect={onSelect} search={search} />
      <div className="flex items-center px-4 h-10 border-t border-border shrink-0">
        <a
          href="https://skills.sh"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
        >
          Browse skills.sh
          <IconExternalLink className="h-3.5 w-3.5" />
        </a>
      </div>
    </div>
  );
}

function SkillListHeader({
  count,
  onRefresh,
  onAdd,
}: {
  count: number;
  onRefresh: () => void;
  onAdd: () => void;
}) {
  return (
    <div className="flex items-center justify-between px-4 pt-4 pb-2">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">Skills</h3>
        <Badge variant="secondary" className="text-xs">
          {count} available
        </Badge>
      </div>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              onClick={onRefresh}
              className="h-7 w-7 p-0 cursor-pointer"
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refresh skills</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              onClick={onAdd}
              className="h-7 w-7 p-0 cursor-pointer"
            >
              <IconPlus className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Create new skill</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

function SkillImportInput({
  value,
  onChange,
  onImport,
  importing,
}: {
  value: string;
  onChange: (v: string) => void;
  onImport: () => void;
  importing: boolean;
}) {
  return (
    <div className="px-4 pb-2">
      <div className="flex gap-1">
        <Input
          placeholder="Path, GitHub URL, or skills.sh"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && onImport()}
          className="h-8 text-xs"
        />
        <Button
          variant="secondary"
          size="sm"
          onClick={onImport}
          disabled={!value.trim() || importing}
          className="h-8 shrink-0 cursor-pointer"
        >
          {importing ? <IconLoader2 className="h-3.5 w-3.5 animate-spin" /> : "Add"}
        </Button>
      </div>
    </div>
  );
}

function SkillItems({
  items,
  selectedId,
  onSelect,
  search,
}: {
  items: Skill[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  search: string;
}) {
  return (
    <div className="flex-1 overflow-y-auto px-2 space-y-0.5">
      {items.length === 0 && (
        <p className="text-sm text-muted-foreground px-3 py-2">
          {search ? "No matching skills" : "No skills yet. Import from GitHub or create your own."}
        </p>
      )}
      {items.map((s) => (
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
          {sourceIcon(s.sourceType)}
        </button>
      ))}
    </div>
  );
}
