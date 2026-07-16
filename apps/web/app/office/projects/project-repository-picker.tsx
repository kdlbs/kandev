"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconCode, IconPlus, IconWorld } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { cn, formatUserHomePath } from "@/lib/utils";
import type { LocalRepository } from "@/lib/types/http";

type Props = {
  workspaceId: string | null;
  /** Already-attached entries (raw stored values) — excluded from suggestions. */
  exclude: string[];
  onSelect: (value: string) => void;
  /** Optional render override for the trigger. Defaults to "+ Add repository". */
  triggerLabel?: string;
};

type RepoOption = { key: string; value: string; name: string; path: string };

/**
 * Popover picker for adding a repository to a project. Mirrors the
 * task-create dialog style (cmdk search, "on disk" badge, workspace
 * group). The project model is a flat `string[]`, so:
 *
 *   - selecting a workspace repo stores its `local_path`;
 *   - selecting a discovered on-disk repo stores its absolute path;
 *   - typing a URL or path that doesn't match anything offers an
 *     "Add custom" row which stores the literal input.
 *
 * On-disk discovery runs lazily the first time the popover opens.
 */
export function ProjectRepositoryPicker({ workspaceId, exclude, onSelect, triggerLabel }: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const { repositories } = useRepositories(workspaceId);
  // `null` means "discovery not yet attempted" — used to drive the
  // "Searching your machine…" empty-state copy without an extra
  // loading flag (the lint rule discourages synchronous setState in
  // an effect body).
  const [discovered, setDiscovered] = useState<LocalRepository[] | null>(null);
  const discoveredOnce = useRef(false);

  useEffect(() => {
    if (!open || !workspaceId || discoveredOnce.current) return;
    discoveredOnce.current = true;
    let cancelled = false;
    discoverRepositoriesAction(workspaceId)
      .then((res) => {
        if (!cancelled) setDiscovered(res.repositories ?? []);
      })
      .catch(() => {
        if (!cancelled) setDiscovered([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId]);

  const excludeSet = useMemo(() => new Set(exclude.map(normalize)), [exclude]);
  const workspaceOptions = useMemo<RepoOption[]>(
    () =>
      repositories
        .filter((r) => r.local_path && !excludeSet.has(normalize(r.local_path)))
        .map((r) => ({
          key: `ws-${r.id}`,
          value: r.local_path,
          name: r.name,
          path: r.local_path,
        })),
    [repositories, excludeSet],
  );
  const discoveredOptions = useMemo<RepoOption[]>(() => {
    if (!discovered) return [];
    const wsPaths = new Set(workspaceOptions.map((o) => normalize(o.path)));
    return discovered
      .filter((r) => !wsPaths.has(normalize(r.path)) && !excludeSet.has(normalize(r.path)))
      .map((r) => ({
        key: `disc-${r.path}`,
        value: r.path,
        name: r.name || leafSegment(r.path),
        path: r.path,
      }));
  }, [discovered, workspaceOptions, excludeSet]);

  const handleSelect = useCallback(
    (value: string) => {
      onSelect(value);
      setOpen(false);
      setQuery("");
    },
    [onSelect],
  );

  const trimmed = query.trim();
  const showCustom =
    trimmed.length > 0 &&
    !excludeSet.has(normalize(trimmed)) &&
    !workspaceOptions.some((o) => matches(o, trimmed)) &&
    !discoveredOptions.some((o) => matches(o, trimmed));

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <PickerTriggerButton label={triggerLabel ?? "Add repository"} />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>
          Pick a workspace repo, a discovered local path, or paste a URL.
        </TooltipContent>
      </Tooltip>
      <PopoverContent className="w-[420px] p-0" align="start" portal={false}>
        <Command>
          <CommandInput
            placeholder="Search or paste a URL or path…"
            value={query}
            onValueChange={setQuery}
            className="h-9"
          />
          <PickerCommandList
            workspaceOptions={workspaceOptions}
            discoveredOptions={discoveredOptions}
            discoveryLoading={discovered === null}
            showCustom={showCustom}
            customQuery={trimmed}
            onSelect={handleSelect}
          />
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function PickerTriggerButton({ label, ...rest }: { label: string }) {
  return (
    <button
      type="button"
      data-testid="project-add-repository"
      className={cn(
        "h-8 inline-flex items-center gap-1.5 rounded-md border border-input bg-input/20 dark:bg-input/30 px-2.5 text-xs cursor-pointer",
        "hover:bg-muted/60",
      )}
      {...rest}
    >
      <IconPlus className="h-3.5 w-3.5" />
      {label}
    </button>
  );
}

function PickerCommandList({
  workspaceOptions,
  discoveredOptions,
  discoveryLoading,
  showCustom,
  customQuery,
  onSelect,
}: {
  workspaceOptions: RepoOption[];
  discoveredOptions: RepoOption[];
  discoveryLoading: boolean;
  showCustom: boolean;
  customQuery: string;
  onSelect: (value: string) => void;
}) {
  return (
    <CommandList>
      <CommandEmpty>{discoveryLoading ? "Searching your machine…" : "No matches."}</CommandEmpty>
      {showCustom && (
        <CommandGroup heading="Add custom">
          <CommandItem
            value={`__custom__:${customQuery}`}
            onSelect={() => onSelect(customQuery)}
            className="cursor-pointer"
            data-testid="project-add-custom"
          >
            <IconWorld className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="flex flex-col min-w-0">
              <span className="truncate">Use “{customQuery}”</span>
              <span className="text-[11px] text-muted-foreground">
                {looksLikeUrl(customQuery) ? "Add as remote URL" : "Add as local path"}
              </span>
            </span>
          </CommandItem>
        </CommandGroup>
      )}
      {workspaceOptions.length > 0 && (
        <RepoGroup heading="Workspace" options={workspaceOptions} onSelect={onSelect} />
      )}
      {discoveredOptions.length > 0 && (
        <RepoGroup
          heading="On disk"
          options={discoveredOptions}
          onSelect={onSelect}
          badge="on disk"
        />
      )}
    </CommandList>
  );
}

function RepoGroup({
  heading,
  options,
  onSelect,
  badge,
}: {
  heading: string;
  options: RepoOption[];
  onSelect: (value: string) => void;
  badge?: string;
}) {
  return (
    <CommandGroup heading={heading}>
      {options.map((o) => (
        <CommandItem
          key={o.key}
          value={o.value}
          keywords={[o.name, o.path, formatUserHomePath(o.path)]}
          onSelect={() => onSelect(o.value)}
          className="cursor-pointer"
        >
          <IconCode className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="flex flex-col min-w-0 flex-1">
            <span className="truncate">{o.name}</span>
            <span className="truncate text-[11px] text-muted-foreground">
              {formatUserHomePath(o.path)}
            </span>
          </span>
          {badge && (
            <Badge variant="outline" className="text-[10px] text-muted-foreground shrink-0">
              {badge}
            </Badge>
          )}
        </CommandItem>
      ))}
    </CommandGroup>
  );
}

function normalize(value: string): string {
  return value.trim().replace(/\\/g, "/").replace(/\/+$/g, "").toLowerCase();
}

function leafSegment(path: string): string {
  const cleaned = path.replace(/\\/g, "/").replace(/\/+$/g, "");
  const idx = cleaned.lastIndexOf("/");
  return idx >= 0 ? cleaned.slice(idx + 1) : cleaned;
}

function matches(option: { name: string; path: string }, query: string): boolean {
  const q = query.toLowerCase();
  return option.name.toLowerCase().includes(q) || option.path.toLowerCase().includes(q);
}

function looksLikeUrl(value: string): boolean {
  return /^(https?:\/\/|git@|ssh:\/\/|git:\/\/)/i.test(value);
}
