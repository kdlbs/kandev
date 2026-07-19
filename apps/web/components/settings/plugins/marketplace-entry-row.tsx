"use client";

import { useState } from "react";
import { IconArrowUpCircle, IconCheck, IconExternalLink, IconStar } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { MarketplaceEntry } from "@/lib/types/plugins";

type MarketplaceEntryRowProps = {
  entry: MarketplaceEntry;
  busy: boolean;
  onInstall: (entry: MarketplaceEntry) => void;
};

/** One catalog card: a neutral tile, metadata, stars, and an install-state action. */
export function MarketplaceEntryRow({ entry, busy, onInstall }: MarketplaceEntryRowProps) {
  return (
    <div
      data-testid={`marketplace-entry-${entry.id}`}
      className="group flex flex-col gap-3 rounded-xl border border-border/60 bg-card p-4 transition-colors hover:border-border"
    >
      <div className="flex items-start gap-3">
        <PluginTile entry={entry} />

        <div className="min-w-0 flex-1 space-y-0.5">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium text-foreground">{entry.name}</span>
            <span className="text-xs text-muted-foreground">v{entry.version}</span>
            {!entry.source_name.includes("Official") && (
              <Badge variant="outline" className="text-[10px]">
                {entry.source_name}
              </Badge>
            )}
          </div>
          {entry.description && (
            <p className="line-clamp-2 text-sm text-muted-foreground">{entry.description}</p>
          )}
        </div>

        <MarketplaceEntryAction entry={entry} busy={busy} onInstall={onInstall} />
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1">
          <IconStar className="h-3.5 w-3.5" />
          {entry.stars.toLocaleString()}
        </span>
        {entry.author && <span>by {entry.author}</span>}
        {entry.categories.map((cat) => (
          <Badge key={cat} variant="secondary" className="text-[10px] font-normal">
            {cat}
          </Badge>
        ))}
        {entry.repo_url && (
          <a
            href={entry.repo_url}
            target="_blank"
            rel="noreferrer"
            className="ml-auto inline-flex items-center gap-1 hover:text-foreground cursor-pointer"
          >
            <IconExternalLink className="h-3.5 w-3.5" />
            Repo
          </a>
        )}
      </div>
    </div>
  );
}

/**
 * The plugin's icon when it ships one, falling back to a neutral letter tile
 * (also used when the icon URL fails to load).
 */
function PluginTile({ entry }: { entry: MarketplaceEntry }) {
  const [failed, setFailed] = useState(false);
  const tileClass =
    "flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border/60 bg-muted";

  if (entry.icon_url && !failed) {
    return (
      <div className={tileClass}>
        <img
          src={entry.icon_url}
          alt=""
          className="h-full w-full object-contain"
          loading="lazy"
          onError={() => setFailed(true)}
        />
      </div>
    );
  }
  return (
    <div className={`${tileClass} text-base font-medium text-muted-foreground`} aria-hidden>
      {(entry.name || entry.id).charAt(0).toUpperCase()}
    </div>
  );
}

function MarketplaceEntryAction({ entry, busy, onInstall }: MarketplaceEntryRowProps) {
  if (entry.install_state === "installed") {
    return (
      <Badge
        variant="secondary"
        data-testid={`marketplace-installed-${entry.id}`}
        className="shrink-0 gap-1 text-muted-foreground"
      >
        <IconCheck className="h-3.5 w-3.5" />
        Installed
      </Badge>
    );
  }
  if (entry.install_state === "update_available") {
    return (
      <Button
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={() => onInstall(entry)}
        data-testid={`marketplace-install-${entry.id}`}
        className="shrink-0 gap-1 cursor-pointer"
      >
        <IconArrowUpCircle className="h-4 w-4" />
        {busy ? "Updating…" : "Update"}
      </Button>
    );
  }
  return (
    <Button
      size="sm"
      variant="outline"
      disabled={busy}
      onClick={() => onInstall(entry)}
      data-testid={`marketplace-install-${entry.id}`}
      className="shrink-0 cursor-pointer"
    >
      {busy ? "Installing…" : "Install"}
    </Button>
  );
}
