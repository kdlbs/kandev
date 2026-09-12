"use client";

import { useEffect, useMemo, useState } from "react";
import { IconLayoutGrid, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useMarketplace } from "@/hooks/domains/plugins/use-marketplace";
import type { CatalogQuery } from "@/lib/api/domains/marketplace-api";
import type { MarketplaceCatalog, MarketplaceEntry } from "@/lib/types/plugins";
import { MarketplaceSourcesDialog } from "./marketplace-sources-dialog";
import { CanvasInstallDialog } from "./canvas-install-dialog";
import { CanvasMarketplaceDetail } from "./canvas-marketplace-detail";

const ALL_CATEGORIES = "__all__";

export function resolveCanvasMarketplaceWorkspaceId(
  currentWorkspaceId: string,
  activeWorkspaceId: string | null | undefined,
  workspaces: ReadonlyArray<{ id: string }>,
): string {
  if (workspaces.some((workspace) => workspace.id === currentWorkspaceId)) {
    return currentWorkspaceId;
  }
  if (activeWorkspaceId && workspaces.some((workspace) => workspace.id === activeWorkspaceId)) {
    return activeWorkspaceId;
  }
  return workspaces[0]?.id ?? "";
}

// eslint-disable-next-line max-lines-per-function, complexity -- One marketplace surface owns filtering, detail, and install transitions.
export function CanvasMarketplace() {
  const { t } = useTranslation();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const canManageSources = useIsAdmin();
  const [workspaceId, setWorkspaceId] = useState(activeWorkspaceId ?? workspaces[0]?.id ?? "");
  const [text, setText] = useState("");
  const [category, setCategory] = useState(ALL_CATEGORIES);
  const [sort, setSort] = useState<CatalogQuery["sort"]>("stars");
  const [selected, setSelected] = useState<MarketplaceEntry | null>(null);
  const [installEntry, setInstallEntry] = useState<MarketplaceEntry | null>(null);
  const [installOpen, setInstallOpen] = useState(false);
  const [sourcesOpen, setSourcesOpen] = useState(false);

  useEffect(() => {
    const nextWorkspaceId = resolveCanvasMarketplaceWorkspaceId(
      workspaceId,
      activeWorkspaceId,
      workspaces,
    );
    if (nextWorkspaceId !== workspaceId) {
      setWorkspaceId(nextWorkspaceId);
    }
  }, [activeWorkspaceId, workspaceId, workspaces]);

  const { catalog, loading, error, reload } = useMarketplace({
    q: text.trim() || undefined,
    sort,
    kind: "canvas",
  });
  const entries = useMemo(() => {
    const candidates =
      catalog.canvases ?? catalog.plugins.filter((entry) => entry.kind === "canvas");
    return category === ALL_CATEGORIES
      ? candidates
      : candidates.filter((entry) => entry.categories.includes(category));
  }, [catalog.canvases, catalog.plugins, category]);
  const categories = useMemo(() => {
    const values = new Set<string>();
    for (const entry of catalog.canvases ??
      catalog.plugins.filter((item) => item.kind === "canvas")) {
      entry.categories.forEach((value) => values.add(value));
    }
    return Array.from(values).sort();
  }, [catalog.canvases, catalog.plugins]);

  if (selected) {
    return (
      <div className="space-y-4">
        <CanvasMarketplaceDetail
          entry={selected}
          onBack={() => setSelected(null)}
          onInstall={() => {
            setInstallEntry(selected);
            setInstallOpen(true);
          }}
        />
        {workspaceId && (
          <CanvasInstallDialog
            open={installOpen}
            onOpenChange={(open) => {
              setInstallOpen(open);
              if (!open) setInstallEntry(null);
            }}
            workspaceId={workspaceId}
            workspaceName={workspaces.find((workspace) => workspace.id === workspaceId)?.name}
            entry={installEntry}
          />
        )}
      </div>
    );
  }

  return (
    <div className="space-y-4" data-testid="canvas-marketplace">
      <div className="flex flex-col gap-3 rounded-xl border border-border/70 bg-card p-4">
        <div className="flex flex-col gap-2 md:flex-row md:items-center">
          <div className="min-w-0 flex-1">
            <h2 className="text-lg font-semibold">{t("plugins:tabCanvases")}</h2>
            <p className="text-sm text-muted-foreground">
              {t("plugins:canvasMarketplaceDescription")}
            </p>
          </div>
          <Button
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-9"
            onClick={() => void reload()}
            disabled={loading}
          >
            <IconRefresh className="mr-1.5 h-4 w-4" />
            {t("plugins:refresh")}
          </Button>
          {canManageSources && (
            <Button
              variant="outline"
              className="min-h-11 cursor-pointer md:min-h-9"
              onClick={() => setSourcesOpen(true)}
            >
              {t("plugins:sources")}
            </Button>
          )}
          <Button
            className="min-h-11 cursor-pointer md:min-h-9"
            onClick={() => {
              setInstallEntry(null);
              setInstallOpen(true);
            }}
          >
            {t("plugins:installCanvas")}
          </Button>
        </div>
        <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(12rem,auto)_minmax(10rem,auto)]">
          <Input
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder={t("plugins:searchCanvases")}
            data-testid="canvas-marketplace-search"
          />
          <Select value={workspaceId || undefined} onValueChange={setWorkspaceId}>
            <SelectTrigger
              className="min-h-11 md:min-h-9"
              data-testid="canvas-marketplace-workspace"
            >
              <SelectValue placeholder={t("plugins:selectWorkspace")} />
            </SelectTrigger>
            <SelectContent>
              {workspaces.map((workspace) => (
                <SelectItem key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex gap-2">
            <Select value={category} onValueChange={setCategory}>
              <SelectTrigger
                className="min-h-11 min-w-0 flex-1 md:min-h-9"
                data-testid="canvas-marketplace-category"
              >
                <SelectValue placeholder={t("plugins:category")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_CATEGORIES}>{t("plugins:allCategories")}</SelectItem>
                {categories.map((value) => (
                  <SelectItem key={value} value={value}>
                    {value}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={sort} onValueChange={(value) => setSort(value as CatalogQuery["sort"])}>
              <SelectTrigger
                className="min-h-11 min-w-0 flex-1 md:min-h-9"
                data-testid="canvas-marketplace-sort"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="stars">{t("plugins:sortMostStars")}</SelectItem>
                <SelectItem value="recent">{t("plugins:sortRecentlyUpdated")}</SelectItem>
                <SelectItem value="name">{t("plugins:sortName")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      <CanvasSourceHealth catalog={catalog} />
      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
          {error}
        </div>
      )}
      {loading && entries.length === 0 && (
        <p
          className="rounded-lg border border-dashed p-6 text-sm text-muted-foreground"
          role="status"
        >
          {t("plugins:loadingCanvasesMarketplace")}
        </p>
      )}
      {!loading && entries.length === 0 && (
        <div className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {text.trim() || category !== ALL_CATEGORIES
            ? t("plugins:noCanvasesMatchSearch")
            : t("plugins:noCanvasesAvailable")}
        </div>
      )}
      {entries.length > 0 && (
        <div className="space-y-3">
          <p className="text-xs text-muted-foreground">
            {t("plugins:canvasesAvailable", { count: entries.length })}
          </p>
          <div className="grid gap-3 md:grid-cols-2">
            {entries.map((entry) => (
              <CanvasMarketplaceCard
                key={`${entry.source_id}:${entry.id}`}
                entry={entry}
                onOpen={() => setSelected(entry)}
                onInstall={() => {
                  setInstallEntry(entry);
                  setInstallOpen(true);
                }}
              />
            ))}
          </div>
        </div>
      )}

      {canManageSources && (
        <MarketplaceSourcesDialog
          open={sourcesOpen}
          sources={catalog.sources}
          onOpenChange={setSourcesOpen}
          onChanged={() => void reload()}
        />
      )}
      {workspaceId && (
        <CanvasInstallDialog
          open={installOpen}
          onOpenChange={(open) => {
            setInstallOpen(open);
            if (!open) setInstallEntry(null);
          }}
          workspaceId={workspaceId}
          workspaceName={workspaces.find((workspace) => workspace.id === workspaceId)?.name}
          entry={installEntry}
        />
      )}
    </div>
  );
}

function CanvasMarketplaceCard({
  entry,
  onOpen,
  onInstall,
}: {
  entry: MarketplaceEntry;
  onOpen: () => void;
  onInstall: () => void;
}) {
  const { t } = useTranslation();
  const cover = entry.previews?.[0];
  const [imageFailed, setImageFailed] = useState(false);
  return (
    <article
      className="overflow-hidden rounded-xl border border-border/70 bg-card"
      data-testid={`canvas-marketplace-entry-${entry.id}`}
    >
      <button type="button" className="block w-full cursor-pointer text-left" onClick={onOpen}>
        <div className="flex aspect-[16/8] items-center justify-center bg-muted/40">
          {cover && !imageFailed ? (
            <img
              src={cover.url}
              alt={cover.alt}
              className="h-full w-full object-cover"
              onError={() => setImageFailed(true)}
            />
          ) : (
            <IconLayoutGrid className="h-10 w-10 text-muted-foreground" aria-hidden="true" />
          )}
        </div>
        <div className="space-y-2 p-4">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="min-w-0 flex-1 truncate font-medium">{entry.name}</h3>
            <Badge variant="secondary">v{entry.version}</Badge>
          </div>
          <p className="line-clamp-2 text-sm text-muted-foreground">{entry.description}</p>
          <p className="text-xs text-muted-foreground">
            {t("plugins:byAuthor", { author: entry.author })}
          </p>
        </div>
      </button>
      <div className="flex items-center justify-between gap-2 border-t px-4 py-3">
        <span className="text-xs text-muted-foreground">{entry.source_name}</span>
        <Button
          className="min-h-11 cursor-pointer md:min-h-9"
          variant="outline"
          onClick={onInstall}
          disabled={entry.install_state === "installed"}
        >
          {entry.install_state === "installed"
            ? t("plugins:installed")
            : t("plugins:installCanvas")}
        </Button>
      </div>
    </article>
  );
}

function CanvasSourceHealth({ catalog }: { catalog: MarketplaceCatalog }) {
  const { t } = useTranslation();
  const degraded = catalog.sources.filter((source) => source.enabled && source.healthy === false);
  if (degraded.length === 0) return null;
  return (
    <div
      className="space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400"
      data-testid="canvas-marketplace-degraded-sources"
    >
      {degraded.map((source) => (
        <p key={source.id}>
          {t("plugins:canvasSourceUnreachable", {
            name: source.name,
            detail: source.error ? `: ${source.error}` : "",
          })}
        </p>
      ))}
    </div>
  );
}
