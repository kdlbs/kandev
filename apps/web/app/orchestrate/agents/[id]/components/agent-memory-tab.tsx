"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  IconSearch,
  IconTrash,
  IconDownload,
  IconChevronDown,
  IconChevronRight,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@kandev/ui/dialog";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";

type MemoryEntry = {
  id: string;
  layer: string;
  key: string;
  content: string;
  metadata: string;
  updated_at: string;
};

type AgentMemoryTabProps = {
  agent: AgentInstance;
};

export function AgentMemoryTab({ agent }: AgentMemoryTabProps) {
  const [entries, setEntries] = useState<MemoryEntry[]>([]);
  const [search, setSearch] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [clearDialogOpen, setClearDialogOpen] = useState(false);
  const [collapsedLayers, setCollapsedLayers] = useState<Set<string>>(new Set());

  useEffect(() => {
    let cancelled = false;
    orchestrateApi.getMemory(agent.id).then((res) => {
      if (!cancelled) {
        setEntries((res as { memory?: MemoryEntry[] }).memory ?? []);
      }
    }).catch(() => { /* ignore */ });
    return () => { cancelled = true; };
  }, [agent.id]);

  const filtered = useMemo(() => {
    if (!search) return entries;
    const q = search.toLowerCase();
    return entries.filter(
      (e) => e.key.toLowerCase().includes(q) || e.content.toLowerCase().includes(q),
    );
  }, [entries, search]);

  const grouped = useMemo(() => {
    const groups: Record<string, MemoryEntry[]> = { operating: [], knowledge: [], session: [] };
    for (const e of filtered) {
      const layer = e.layer in groups ? e.layer : "knowledge";
      groups[layer].push(e);
    }
    return groups;
  }, [filtered]);

  const handleDelete = useCallback(
    async (entryId: string) => {
      await orchestrateApi.deleteMemory(agent.id, entryId);
      setEntries((prev) => prev.filter((e) => e.id !== entryId));
    },
    [agent.id],
  );

  const handleClearAll = useCallback(async () => {
    await orchestrateApi.deleteAllMemory(agent.id);
    setEntries([]);
    setClearDialogOpen(false);
  }, [agent.id]);

  const handleExport = useCallback(async () => {
    const res = await orchestrateApi.exportMemory(agent.id);
    const data = (res as { memory?: MemoryEntry[] }).memory ?? [];
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${agent.name}-memory.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [agent.id, agent.name]);

  const toggleLayer = useCallback((layer: string) => {
    setCollapsedLayers((prev) => {
      const next = new Set(prev);
      if (next.has(layer)) next.delete(layer);
      else next.add(layer);
      return next;
    });
  }, []);

  return (
    <div className="mt-4 space-y-4">
      <MemoryToolbar
        search={search}
        onSearchChange={setSearch}
        onClearAll={() => setClearDialogOpen(true)}
        onExport={handleExport}
        isEmpty={entries.length === 0}
      />

      {entries.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <p className="text-sm text-muted-foreground">No memory entries yet.</p>
        </div>
      ) : (
        <div className="space-y-4">
          {(["operating", "knowledge", "session"] as const).map((layer) => (
            <MemoryLayerGroup
              key={layer}
              layer={layer}
              entries={grouped[layer] ?? []}
              collapsed={collapsedLayers.has(layer)}
              expandedId={expandedId}
              onToggleLayer={() => toggleLayer(layer)}
              onToggleEntry={(id) => setExpandedId(expandedId === id ? null : id)}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      <ClearAllDialog
        open={clearDialogOpen}
        onOpenChange={setClearDialogOpen}
        onConfirm={handleClearAll}
      />
    </div>
  );
}

function MemoryToolbar({
  search,
  onSearchChange,
  onClearAll,
  onExport,
  isEmpty,
}: {
  search: string;
  onSearchChange: (v: string) => void;
  onClearAll: () => void;
  onExport: () => void;
  isEmpty: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      <div className="relative flex-1 max-w-[300px]">
        <IconSearch className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search memory..."
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          className="pl-8 h-9 text-sm"
        />
      </div>
      <div className="ml-auto flex items-center gap-1">
        <Button
          variant="outline"
          size="sm"
          onClick={onExport}
          disabled={isEmpty}
          className="cursor-pointer"
        >
          <IconDownload className="h-4 w-4 mr-1" />
          Export
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={onClearAll}
          disabled={isEmpty}
          className="cursor-pointer"
        >
          Clear All
        </Button>
      </div>
    </div>
  );
}

function MemoryLayerGroup({
  layer,
  entries,
  collapsed,
  expandedId,
  onToggleLayer,
  onToggleEntry,
  onDelete,
}: {
  layer: string;
  entries: MemoryEntry[];
  collapsed: boolean;
  expandedId: string | null;
  onToggleLayer: () => void;
  onToggleEntry: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  if (entries.length === 0) return null;

  const Icon = collapsed ? IconChevronRight : IconChevronDown;

  return (
    <div>
      <button
        onClick={onToggleLayer}
        className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-widest text-muted-foreground/60 mb-2 cursor-pointer"
      >
        <Icon className="h-3.5 w-3.5" />
        {layer} ({entries.length})
      </button>
      {!collapsed && (
        <div className="border border-border rounded-lg divide-y divide-border">
          {entries.map((entry) => (
            <MemoryEntryRow
              key={entry.id}
              entry={entry}
              expanded={expandedId === entry.id}
              onToggle={() => onToggleEntry(entry.id)}
              onDelete={() => onDelete(entry.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function MemoryEntryRow({
  entry,
  expanded,
  onToggle,
  onDelete,
}: {
  entry: MemoryEntry;
  expanded: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="px-4 py-2.5">
      <div
        className="flex items-center gap-2 text-sm cursor-pointer"
        onClick={onToggle}
      >
        <Badge variant="outline" className="text-[10px] font-mono shrink-0">
          {entry.key}
        </Badge>
        <span className="flex-1 truncate text-muted-foreground">
          {entry.content.slice(0, 80)}
          {entry.content.length > 80 ? "..." : ""}
        </span>
        <span className="text-xs text-muted-foreground shrink-0">
          {entry.updated_at ? new Date(entry.updated_at).toLocaleDateString() : ""}
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
      {expanded && (
        <pre className="mt-2 text-xs whitespace-pre-wrap bg-muted/50 rounded p-3">
          {entry.content}
        </pre>
      )}
    </div>
  );
}

function ClearAllDialog({
  open,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Clear all memory?</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          This will permanently delete all memory entries for this agent. This action cannot be
          undone.
        </p>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button variant="destructive" onClick={onConfirm} className="cursor-pointer">
            Clear All
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
