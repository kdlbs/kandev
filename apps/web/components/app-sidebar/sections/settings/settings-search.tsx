"use client";

import { IconSearch, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import type { ChangeEvent, KeyboardEvent, MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { navigateToSettingsDiscovery } from "@/lib/settings-discovery/navigation";
import { searchSettingsDiscovery } from "@/lib/settings-discovery/search";
import type { ResolvedSettingsDiscoveryItem } from "@/lib/settings-discovery/types";
import { useSettingsSearchMotion } from "./use-settings-search-motion";

type SettingsSearchProps = {
  items: ResolvedSettingsDiscoveryItem[];
  query: string;
  onQueryChange: (query: string) => void;
  onSelect: () => void;
  /**
   * Where the field sits.
   *
   * - `inline` — sticky at the top of the tree, which is where the desktop
   *   sidebar wants it: above the list it filters.
   * - `floating` — pinned near the bottom of the viewport for the `/settings`
   *   index on a phone, inside thumb reach. Results still render in the page
   *   flow above it, so the list reads top-down while the field stays put.
   */
  layout?: "inline" | "floating";
};

/**
 * Where the floating field sits.
 *
 * Bottom clears the status bar and the home-indicator inset.
 *
 * The side insets are equal, which keeps the field centred, and each is wide
 * enough to clear the config-chat button in the same corner: that button is
 * `right-6 w-12`, so 4.5rem of occupied edge, plus a gap. Mirroring it on the
 * left costs width rather than centring — on a phone the field has room to
 * spare, and a centred field that never collides beats a wider offset one.
 */
const FLOATING_POSITION =
  "bottom-[calc(0.75rem+env(safe-area-inset-bottom)+var(--app-status-bar-height))] left-20 right-20 mx-auto max-w-[26rem]";

type ResultGroup = {
  id: string;
  label: string;
  items: ResolvedSettingsDiscoveryItem[];
};

export function SettingsSearch({
  items,
  query,
  onQueryChange,
  onSelect,
  layout = "inline",
}: SettingsSearchProps) {
  const { t } = useTranslation();
  const trimmedQuery = query.trim();
  const results = trimmedQuery ? searchSettingsDiscovery(items, trimmedQuery) : [];
  const floating = layout === "floating";

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Escape" || query.length === 0) return;
    event.preventDefault();
    event.stopPropagation();
    onQueryChange("");
  };

  return (
    <>
      <div
        className={
          floating
            ? `fixed z-30 rounded-xl border border-border bg-background p-1 shadow-lg shadow-black/20 ${FLOATING_POSITION}`
            : "sticky top-0 z-10 bg-background pb-2 md:bg-sidebar md:pb-1.5"
        }
        data-testid="settings-search"
      >
        <div className="relative">
          <IconSearch
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground md:left-2.5 md:size-3.5"
          />
          <Input
            type="search"
            value={query}
            aria-label={t("settings:searchSettings")}
            placeholder={t("settings:searchSettings")}
            autoComplete="off"
            spellCheck={false}
            onChange={(event: ChangeEvent<HTMLInputElement>) => onQueryChange(event.target.value)}
            onKeyDown={handleKeyDown}
            className={
              floating
                ? "h-11 appearance-none border-0 bg-transparent pl-9 pr-11 text-base shadow-none [&::-webkit-search-cancel-button]:hidden"
                : "h-11 appearance-none bg-background pl-9 pr-11 text-base md:h-8 md:pl-8 md:pr-8 md:text-xs [&::-webkit-search-cancel-button]:hidden"
            }
          />
          {query && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("settings:clearSettingsSearch")}
              onClick={() => onQueryChange("")}
              className="absolute right-0 top-0 size-11 text-muted-foreground hover:text-foreground md:size-8"
            >
              <IconX className="size-4 md:size-3.5" />
            </Button>
          )}
        </div>
      </div>
      {trimmedQuery && <SettingsSearchResults results={results} onSelect={onSelect} />}
    </>
  );
}

function SettingsSearchResults({
  results,
  onSelect,
}: {
  results: ResolvedSettingsDiscoveryItem[];
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const groups = groupResults(results);
  const layoutKey = groups
    .flatMap((group) => [`group:${group.id}`, ...group.items.map((item) => `item:${item.id}`)])
    .join("\0");
  const motionRef = useSettingsSearchMotion(layoutKey);

  if (results.length === 0) {
    return (
      <div ref={motionRef}>
        <p
          className="flex min-h-11 items-center px-2.5 text-xs text-muted-foreground"
          role="status"
        >
          {t("settings:noMatchingSettings")}
        </p>
      </div>
    );
  }

  return (
    <div ref={motionRef} data-testid="settings-search-results">
      <p className="sr-only" role="status">
        {t("settings:settingsSearchResults", { count: results.length })}
      </p>
      {groups.map((group) => (
        <section key={group.id} aria-labelledby={`settings-search-group-${group.id}`}>
          <h3
            id={`settings-search-group-${group.id}`}
            data-settings-search-motion-key={`group:${group.id}`}
            className="px-2.5 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground"
          >
            {group.label}
          </h3>
          <div className="flex flex-col gap-0.5">
            {group.items.map((item) => (
              <SettingsSearchResult key={item.id} item={item} onSelect={onSelect} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function SettingsSearchResult({
  item,
  onSelect,
}: {
  item: ResolvedSettingsDiscoveryItem;
  onSelect: () => void;
}) {
  const router = useRouter();
  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (!isPlainPrimaryClick(event)) return;
    event.preventDefault();
    navigateToSettingsDiscovery(item.href, router.push);
    onSelect();
  };

  return (
    <Link
      href={item.href}
      onClick={handleClick}
      data-settings-search-motion-key={`item:${item.id}`}
      className="flex min-h-11 min-w-0 cursor-pointer flex-col justify-center rounded-md px-2.5 py-1.5 outline-none transition-colors hover:bg-muted/70 focus-visible:ring-2 focus-visible:ring-primary/40 motion-reduce:transition-none"
    >
      <span className="truncate text-[13px] font-medium text-foreground">{item.label}</span>
      {item.breadcrumb.length > 0 && (
        <span className="truncate text-[11px] leading-4 text-muted-foreground">
          {item.breadcrumb.join(" › ")}
        </span>
      )}
    </Link>
  );
}

function groupResults(results: ResolvedSettingsDiscoveryItem[]): ResultGroup[] {
  const groups = new Map<string, ResultGroup>();
  for (const item of results) {
    const group = groups.get(item.groupId) ?? {
      id: item.groupId,
      label: item.groupLabel,
      items: [],
    };
    group.items.push(item);
    groups.set(item.groupId, group);
  }
  return [...groups.values()];
}

function isPlainPrimaryClick(event: MouseEvent<HTMLAnchorElement>): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    (!event.currentTarget.target || event.currentTarget.target === "_self")
  );
}
