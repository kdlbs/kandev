"use client";

import { useState } from "react";
import { IconTrash } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Badge } from "@kandev/ui/badge";
import {
  addMarketplaceSource,
  deleteMarketplaceSource,
  updateMarketplaceSource,
} from "@/lib/api/domains/marketplace-api";
import type { MarketplaceSource } from "@/lib/types/plugins";

type MarketplaceSourcesDialogProps = {
  open: boolean;
  sources: MarketplaceSource[];
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
};

/**
 * Manage marketplace sources: the built-in kandev source (toggle only) plus
 * operator-added team/corporate registries (add / toggle / delete). Every
 * mutation calls onChanged so the catalog re-fetches.
 */
export function MarketplaceSourcesDialog({
  open,
  sources,
  onOpenChange,
  onChanged,
}: MarketplaceSourcesDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Marketplace sources</DialogTitle>
          <DialogDescription>
            Add a team or corporate registry to browse its plugins alongside the official catalog.
            Each source serves an index.json document.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          {sources.map((source) => (
            <SourceItem key={source.id} source={source} onChanged={onChanged} />
          ))}
        </div>

        <AddSourceForm onChanged={onChanged} />
      </DialogContent>
    </Dialog>
  );
}

function SourceItem({ source, onChanged }: { source: MarketplaceSource; onChanged: () => void }) {
  const [busy, setBusy] = useState(false);

  const toggle = async (enabled: boolean) => {
    setBusy(true);
    try {
      await updateMarketplaceSource(source.id, { enabled });
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update source");
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await deleteMarketplaceSource(source.id);
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to remove source");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      data-testid={`marketplace-source-${source.id}`}
      className="flex items-center justify-between gap-2 rounded-md border border-border/60 p-3"
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">{source.name}</span>
          {source.builtin && (
            <Badge variant="outline" className="text-[10px]">
              Official
            </Badge>
          )}
          {source.healthy === false && (
            <Badge variant="destructive" className="text-[10px]">
              Unreachable
            </Badge>
          )}
        </div>
        <span className="text-xs text-muted-foreground truncate block">{source.url}</span>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Switch checked={source.enabled} disabled={busy} onCheckedChange={toggle} />
        {!source.builtin && (
          <Button
            variant="ghost"
            size="icon"
            disabled={busy}
            onClick={remove}
            className="cursor-pointer"
            aria-label={`Remove ${source.name}`}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );
}

function AddSourceForm({ onChanged }: { onChanged: () => void }) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!url.trim()) return;
    setBusy(true);
    try {
      await addMarketplaceSource(name.trim(), url.trim());
      setName("");
      setUrl("");
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add source");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-2 border-t border-border/60 pt-3">
      <Input
        placeholder="Source name (e.g. Acme Internal)"
        value={name}
        onChange={(e) => setName(e.target.value)}
        data-testid="marketplace-add-source-name"
      />
      <div className="flex items-center gap-2">
        <Input
          placeholder="https://.../index.json"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          data-testid="marketplace-add-source-url"
        />
        <Button
          disabled={busy || !url.trim()}
          onClick={submit}
          className="cursor-pointer shrink-0"
          data-testid="marketplace-add-source-submit"
        >
          Add
        </Button>
      </div>
    </div>
  );
}
