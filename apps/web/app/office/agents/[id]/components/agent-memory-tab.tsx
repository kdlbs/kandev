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
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "@/lib/toast/sonner";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import * as officeApi from "@/lib/api/domains/office-api";
import { useTranslation } from "react-i18next";

type MemoryEntry = {
  id: string;
  layer: string;
  key: string;
  content: string;
  metadata: string;
  updated_at: string;
};

type AgentMemoryTabProps = {
  agent: AgentProfile;
};

function useMemoryActions(agent: AgentProfile) {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<MemoryEntry[]>([]);

  useEffect(() => {
    let cancelled = false;
    officeApi
      .getMemory(agent.id)
      .then((res) => {
        if (!cancelled) {
          setEntries((res as { memory?: MemoryEntry[] }).memory ?? []);
        }
      })
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : t("office:failedToLoadMemory"));
      });
    return () => {
      cancelled = true;
    };
  }, [agent.id]);

  const handleDelete = useCallback(
    async (entryId: string) => {
      try {
        await officeApi.deleteMemory(agent.id, entryId);
        setEntries((prev) => prev.filter((e) => e.id !== entryId));
        toast.success(t("office:memoryEntryDeleted"));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("office:failedToDeleteMemoryEntry"));
      }
    },
    [agent.id],
  );

  const handleClearAll = useCallback(async () => {
    try {
      await officeApi.deleteAllMemory(agent.id);
      setEntries([]);
      toast.success(t("office:allMemoryCleared"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToClearMemory"));
    }
  }, [agent.id]);

  const handleExport = useCallback(async () => {
    const res = await officeApi.exportMemory(agent.id);
    const data = (res as { memory?: MemoryEntry[] }).memory ?? [];
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${agent.name}-memory.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [agent.id, agent.name]);

  return { entries, handleDelete, handleClearAll, handleExport };
}

function MemoryGroupsList({
  grouped,
  collapsedLayers,
  expandedId,
  onToggleLayer,
  onToggleEntry,
  onDelete,
}: {
  grouped: Record<string, MemoryEntry[]>;
  collapsedLayers: Set<string>;
  expandedId: string | null;
  onToggleLayer: (layer: string) => void;
  onToggleEntry: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  return (
    <div className="space-y-4">
      {(["operating", "knowledge", "session"] as const).map((layer) => (
        <MemoryLayerGroup
          key={layer}
          layer={layer}
          entries={grouped[layer] ?? []}
          collapsed={collapsedLayers.has(layer)}
          expandedId={expandedId}
          onToggleLayer={() => onToggleLayer(layer)}
          onToggleEntry={onToggleEntry}
          onDelete={onDelete}
        />
      ))}
    </div>
  );
}

function EmptyMemoryState() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-center justify-center py-12 text-center">
      <p className="text-sm text-muted-foreground">{t("office:noMemoryEntriesYet")}</p>
      <p className="text-xs text-muted-foreground mt-1">
        {t("office:memoryIsAccumulatedAsTheAgent")}
      </p>
    </div>
  );
}

export function AgentMemoryTab({ agent }: AgentMemoryTabProps) {
  const [search, setSearch] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [clearDialogOpen, setClearDialogOpen] = useState(false);
  const [collapsedLayers, setCollapsedLayers] = useState<Set<string>>(new Set());
  const { entries, handleDelete, handleClearAll, handleExport } = useMemoryActions(agent);

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
        <EmptyMemoryState />
      ) : (
        <MemoryGroupsList
          grouped={grouped}
          collapsedLayers={collapsedLayers}
          expandedId={expandedId}
          onToggleLayer={toggleLayer}
          onToggleEntry={(id) => setExpandedId(expandedId === id ? null : id)}
          onDelete={handleDelete}
        />
      )}
      <ClearAllDialog
        open={clearDialogOpen}
        onOpenChange={setClearDialogOpen}
        onConfirm={() => {
          handleClearAll();
          setClearDialogOpen(false);
        }}
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <div className="relative flex-1 max-w-[300px]">
        <IconSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={t("office:searchMemory")}
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          className="pl-8"
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
          {t("office:export")}
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={onClearAll}
          disabled={isEmpty}
          className="cursor-pointer"
        >
          {t("office:clearAll")}
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
  const { t } = useTranslation();
  return (
    <div className="px-4 py-2.5">
      <div className="flex items-center gap-2 text-sm cursor-pointer" onClick={onToggle}>
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
        <Tooltip>
          <TooltipTrigger asChild>
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
          </TooltipTrigger>
          <TooltipContent>{t("office:deleteEntry")}</TooltipContent>
        </Tooltip>
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
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("office:clearAllMemory")}</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {t("office:thisWillPermanentlyDeleteAllMemory")}
        </p>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button variant="destructive" onClick={onConfirm} className="cursor-pointer">
            {t("office:clearAll")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
