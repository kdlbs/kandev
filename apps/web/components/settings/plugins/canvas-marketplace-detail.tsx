"use client";

import { IconArrowLeft } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { MarketplacePreviewGallery } from "./marketplace-preview-gallery";
import { PluginRepoLink } from "./plugin-repo-link";

export function CanvasMarketplaceDetail({
  entry,
  onBack,
  onInstall,
}: {
  entry: MarketplaceEntry;
  onBack: () => void;
  onInstall: () => void;
}) {
  const { t } = useTranslation();
  const permissions = entry.permissions;
  const groups = [
    [t("canvases:permissionReads"), permissions?.reads ?? []],
    [t("canvases:permissionWrites"), permissions?.writes ?? []],
    [t("canvases:permissionEvents"), permissions?.events ?? []],
    [t("canvases:permissionExternalOrigins"), permissions?.external_origins ?? []],
  ] as const;
  return (
    <div className="space-y-5" data-testid={`canvas-marketplace-detail-${entry.id}`}>
      <Button variant="ghost" className="cursor-pointer" onClick={onBack}>
        <IconArrowLeft className="mr-2 h-4 w-4" />
        {t("plugins:backToCanvases")}
      </Button>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(18rem,1fr)]">
        <MarketplacePreviewGallery previews={entry.previews} />
        <div className="space-y-4">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-xl font-semibold">{entry.name}</h2>
              <Badge variant="secondary">v{entry.version}</Badge>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("plugins:byAuthor", { author: entry.author })}
            </p>
          </div>
          <p className="text-sm leading-6 text-muted-foreground">{entry.description}</p>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">{t("plugins:license")}</dt>
            <dd>{entry.license || t("plugins:notSet")}</dd>
            <dt className="text-muted-foreground">{t("plugins:requiresVersion")}</dt>
            <dd>{entry.min_kandev_version || t("plugins:notSet")}</dd>
          </dl>
          <PluginRepoLink url={entry.repo_url} className="inline-flex" />
          <Button
            className="w-full cursor-pointer"
            onClick={onInstall}
            disabled={entry.install_state === "installed"}
          >
            {entry.install_state === "installed"
              ? t("plugins:installed")
              : t("plugins:reviewAndInstall")}
          </Button>
        </div>
      </div>
      <section className="space-y-3 rounded-xl border border-border/70 p-4">
        <h3 className="font-medium">{t("plugins:permissions")}</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          {groups.map(([label, values]) => (
            <div key={label} className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">{label}</p>
              <p className="text-sm">{values.length ? values.join(", ") : t("plugins:none")}</p>
            </div>
          ))}
        </div>
        {permissions?.shared_state && <p className="text-sm">{t("canvases:sharedState")}</p>}
      </section>
    </div>
  );
}
