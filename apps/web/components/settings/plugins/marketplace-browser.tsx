"use client";

import { useMemo, useState } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useMarketplace } from "@/hooks/domains/plugins/use-marketplace";
import type { CatalogQuery } from "@/lib/api/domains/marketplace-api";
import type { MarketplaceCatalog, MarketplaceEntry } from "@/lib/types/plugins";
import { MarketplaceEntryRow } from "./marketplace-entry-row";
import { MarketplaceSourcesDialog } from "./marketplace-sources-dialog";

const ALL_CATEGORIES = "__all__";

type MarketplaceBrowserProps = {
  /** Installs a plugin from its package URL; resolves when the install settles. */
  onInstallUrl: (url: string) => Promise<void>;
};

/** The "Browse" tab: search/filter/sort the catalog and install from it. */
export function MarketplaceBrowser({ onInstallUrl }: MarketplaceBrowserProps) {
  const [text, setText] = useState("");
  const [category, setCategory] = useState<string>(ALL_CATEGORIES);
  const [sort, setSort] = useState<CatalogQuery["sort"]>("stars");
  const [sourcesOpen, setSourcesOpen] = useState(false);
  const [installingId, setInstallingId] = useState<string | null>(null);

  const query: CatalogQuery = {
    q: text.trim() || undefined,
    category: category === ALL_CATEGORIES ? undefined : category,
    sort,
  };
  const { catalog, loading, error, reload, softReload } = useMarketplace(query);

  const install = async (entry: MarketplaceEntry) => {
    setInstallingId(entry.id);
    try {
      await onInstallUrl(entry.package_url);
      softReload();
    } finally {
      setInstallingId(null);
    }
  };

  return (
    <div className="space-y-4">
      <MarketplaceToolbar
        text={text}
        onText={setText}
        category={category}
        onCategory={setCategory}
        categories={useCatalogCategories(catalog)}
        sort={sort}
        onSort={setSort}
        onManageSources={() => setSourcesOpen(true)}
        onRefresh={reload}
      />

      <DegradedSourcesBanner catalog={catalog} />

      <MarketplaceList
        catalog={catalog}
        loading={loading}
        error={error}
        filtered={Boolean(query.q || query.category)}
        installingId={installingId}
        onInstall={install}
      />

      <MarketplaceSourcesDialog
        open={sourcesOpen}
        sources={catalog.sources}
        onOpenChange={setSourcesOpen}
        onChanged={reload}
      />
    </div>
  );
}

function useCatalogCategories(catalog: MarketplaceCatalog): string[] {
  return useMemo(() => {
    const set = new Set<string>();
    for (const entry of catalog.plugins) {
      for (const cat of entry.categories) set.add(cat);
    }
    return Array.from(set).sort();
  }, [catalog.plugins]);
}

type ToolbarProps = {
  text: string;
  onText: (v: string) => void;
  category: string;
  onCategory: (v: string) => void;
  categories: string[];
  sort: CatalogQuery["sort"];
  onSort: (v: CatalogQuery["sort"]) => void;
  onManageSources: () => void;
  onRefresh: () => void;
};

function MarketplaceToolbar(props: ToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        placeholder="Search plugins…"
        value={props.text}
        onChange={(e) => props.onText(e.target.value)}
        className="max-w-xs"
        data-testid="marketplace-search"
      />
      <Select value={props.category} onValueChange={props.onCategory}>
        <SelectTrigger className="w-40" data-testid="marketplace-category">
          <SelectValue placeholder="Category" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_CATEGORIES}>All categories</SelectItem>
          {props.categories.map((cat) => (
            <SelectItem key={cat} value={cat}>
              {cat}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={props.sort} onValueChange={(v) => props.onSort(v as CatalogQuery["sort"])}>
        <SelectTrigger className="w-36" data-testid="marketplace-sort">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="stars">Most stars</SelectItem>
          <SelectItem value="recent">Recently updated</SelectItem>
          <SelectItem value="name">Name</SelectItem>
        </SelectContent>
      </Select>
      <div className="ml-auto flex items-center gap-2">
        <Button variant="secondary" onClick={props.onRefresh} className="cursor-pointer">
          <IconRefresh className="h-4 w-4" />
          Refresh
        </Button>
        <Button
          variant="outline"
          onClick={props.onManageSources}
          className="cursor-pointer"
          data-testid="marketplace-manage-sources"
        >
          Sources
        </Button>
      </div>
    </div>
  );
}

function DegradedSourcesBanner({ catalog }: { catalog: MarketplaceCatalog }) {
  const degraded = catalog.sources.filter((s) => s.enabled && s.healthy === false);
  if (degraded.length === 0) return null;
  return (
    <div
      data-testid="marketplace-degraded-sources"
      className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400 space-y-1"
    >
      {degraded.map((s) => (
        <div key={s.id}>
          <span className="font-medium">{s.name}</span> is unreachable
          {s.error ? `: ${s.error}` : ""} — its plugins are hidden.
        </div>
      ))}
    </div>
  );
}

type ListProps = {
  catalog: MarketplaceCatalog;
  loading: boolean;
  error: string | null;
  filtered: boolean;
  installingId: string | null;
  onInstall: (entry: MarketplaceEntry) => void;
};

function MarketplaceList({
  catalog,
  loading,
  error,
  filtered,
  installingId,
  onInstall,
}: ListProps) {
  if (error) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-6 text-sm text-destructive">
        {error}
      </div>
    );
  }
  if (loading && catalog.plugins.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 p-6 text-sm text-muted-foreground">
        Loading marketplace…
      </div>
    );
  }
  if (catalog.plugins.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border/70 p-10 text-center text-sm text-muted-foreground">
        {filtered
          ? "No plugins match your search. Try clearing the filters."
          : "No plugins available yet. Check back soon, or add a marketplace source with your own."}
      </div>
    );
  }
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        {catalog.plugins.length} {catalog.plugins.length === 1 ? "plugin" : "plugins"} available
      </p>
      <div className="grid gap-3 md:grid-cols-2">
        {catalog.plugins.map((entry) => (
          <MarketplaceEntryRow
            key={`${entry.source_id}:${entry.id}`}
            entry={entry}
            busy={installingId === entry.id}
            onInstall={onInstall}
          />
        ))}
      </div>
    </div>
  );
}
