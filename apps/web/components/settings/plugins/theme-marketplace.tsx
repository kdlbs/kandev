"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useMarketplace } from "@/hooks/domains/plugins/use-marketplace";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { ThemeImportDialog } from "./theme-import-dialog";

export function ThemeMarketplace() {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const [entry, setEntry] = useState<MarketplaceEntry | null>(null);
  const { catalog, loading, error, softReload, reload } = useMarketplace({
    kind: "theme",
    q: text || undefined,
    sort: "name",
  });
  return (
    <section className="min-w-0 space-y-4" data-testid="theme-marketplace">
      <div className="flex flex-wrap gap-2">
        <Input
          aria-label={t("common:searchEllipsis")}
          placeholder={t("common:searchEllipsis")}
          value={text}
          onChange={(event) => setText(event.target.value)}
          className="min-h-11 min-w-0 flex-1"
        />
        <Button variant="outline" className="min-h-11 cursor-pointer" onClick={() => void reload()}>
          {t("plugins:refresh")}
        </Button>
        {canManage && (
          <Button
            data-testid="theme-import-trigger"
            className="min-h-11 cursor-pointer"
            onClick={() => {
              setEntry(null);
              setOpen(true);
            }}
          >
            {t("plugins:themeImport")}
          </Button>
        )}
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {loading ? (
        <p className="text-sm text-muted-foreground">{t("common:loading")}</p>
      ) : (
        !catalog.themes?.length && (
          <p className="text-sm text-muted-foreground">{t("plugins:themeEmpty")}</p>
        )
      )}
      <div className="grid min-w-0 gap-3 md:grid-cols-2">
        {catalog.themes?.map((item) => (
          <article key={item.id} className="min-w-0 space-y-2 rounded-lg border p-4">
            <h3 className="break-words font-medium">{item.name}</h3>
            <p className="break-words text-sm text-muted-foreground">{item.description}</p>
            <p className="text-xs">
              {item.author} · {item.license} · {item.version}
            </p>
            <p className="text-xs text-muted-foreground">{item.source_name}</p>
            {canManage && (
              <Button
                variant="outline"
                className="min-h-11 cursor-pointer"
                disabled={item.install_state === "installed"}
                onClick={() => {
                  setEntry(item);
                  setOpen(true);
                }}
              >
                {item.install_state === "installed"
                  ? t("plugins:tabInstalled")
                  : t("plugins:themeReview")}
              </Button>
            )}
          </article>
        ))}
      </div>
      {canManage && (
        <ThemeImportDialog
          open={open}
          onOpenChange={setOpen}
          entry={entry}
          onInstalled={softReload}
        />
      )}
    </section>
  );
}
