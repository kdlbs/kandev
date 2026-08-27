"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
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
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="marketplace-sources-dialog"
        data-layout="contained"
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>{t("plugins:marketplaceSources")}</DialogTitle>
          <DialogDescription>
            {/* `index.json` is the document name a source must serve. */}
            {t("plugins:marketplaceSourcesDescription", { document: "index.json" })}
          </DialogDescription>
        </DialogHeader>

        <div
          data-testid="marketplace-sources-list"
          className="min-h-0 min-w-0 space-y-2 overflow-x-hidden overflow-y-auto overscroll-contain"
        >
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
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);

  const toggle = async (enabled: boolean) => {
    setBusy(true);
    try {
      await updateMarketplaceSource(source.id, { enabled });
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("plugins:failedToUpdateSource"));
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
      toast.error(err instanceof Error ? err.message : t("plugins:failedToRemoveSource"));
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
              {t("plugins:sourceOfficial")}
            </Badge>
          )}
          {source.healthy === false && (
            <Badge variant="destructive" className="text-[10px]">
              {t("plugins:sourceUnreachable")}
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
            className="min-h-11 min-w-11 cursor-pointer sm:min-h-0 sm:min-w-0"
            aria-label={t("plugins:removeSource", { name: source.name })}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );
}

function AddSourceForm({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation();
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
      toast.error(err instanceof Error ? err.message : t("plugins:failedToAddSource"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      data-testid="marketplace-add-source-form"
      className="space-y-2 border-t border-border/60 pt-3"
    >
      <Input
        placeholder={t("plugins:sourceNamePlaceholder")}
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="min-h-11 sm:min-h-7"
        data-testid="marketplace-add-source-name"
      />
      <div className="flex items-center gap-2">
        <Input
          placeholder={t("plugins:sourceUrlPlaceholder")}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          className="min-h-11 sm:min-h-7"
          data-testid="marketplace-add-source-url"
        />
        <Button
          disabled={busy || !url.trim()}
          onClick={submit}
          className="min-h-11 cursor-pointer shrink-0 sm:min-h-0"
          data-testid="marketplace-add-source-submit"
        >
          {t("plugins:add")}
        </Button>
      </div>
    </div>
  );
}
