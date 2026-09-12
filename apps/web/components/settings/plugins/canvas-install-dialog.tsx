"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useCanvasInstall } from "@/hooks/domains/canvas/use-canvas-install";
import { canvasHref } from "@/lib/api/domains/canvas-api";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { MarketplacePreviewGallery } from "./marketplace-preview-gallery";

type InstallMode = "upload" | "url";

// i18n-exempt: package extension filter, not user-facing copy.
const CANVAS_BUNDLE_ACCEPT = ".tar.gz,.tgz";

// eslint-disable-next-line max-lines-per-function, complexity -- One responsive review flow owns its preparation and confirmation states.
export function CanvasInstallDialog({
  open,
  onOpenChange,
  workspaceId,
  workspaceName,
  entry,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  workspaceName?: string;
  entry: MarketplaceEntry | null;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [mode, setMode] = useState<InstallMode>(entry ? "url" : "upload");
  const [file, setFile] = useState<File | null>(null);
  const [url, setUrl] = useState("");
  const install = useCanvasInstall(workspaceId);

  useEffect(() => {
    if (!open) {
      install.reset();
      return;
    }
    install.invalidate();
    setMode(entry ? "url" : "upload");
    setFile(null);
    setUrl("");
    install.reset();
  }, [entry?.id, entry?.version, open, install.invalidate, install.reset]);

  useEffect(() => {
    if (open) install.invalidate();
  }, [open, workspaceId, install.invalidate]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) void install.cancel();
    onOpenChange(nextOpen);
  };

  const close = () => handleOpenChange(false);

  const updateMode = (nextMode: InstallMode) => {
    install.invalidate();
    setMode(nextMode);
  };

  const updateFile = (nextFile: File | null) => {
    install.invalidate();
    setFile(nextFile);
  };

  const updateUrl = (nextUrl: string) => {
    install.invalidate();
    setUrl(nextUrl);
  };

  const inspect = async () => {
    if (entry) {
      await install.prepareCatalog({
        source_id: entry.source_id,
        package_id: entry.id,
        expected_version: entry.version,
        expected_sha256: entry.package_sha256,
        repository_url: entry.repo_url,
      });
      return;
    }
    if (mode === "upload" && file) {
      await install.prepareUpload(file);
      return;
    }
    if (mode === "url" && url.trim()) await install.prepareUrl(url.trim());
  };

  const body = (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4">
      {entry ? (
        <div className="space-y-3 rounded-lg border border-border/70 p-3">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="truncate font-medium">{entry.name}</p>
              <p className="text-xs text-muted-foreground">
                {entry.id} · v{entry.version}
              </p>
            </div>
            <span className="text-xs text-muted-foreground">{entry.source_name}</span>
          </div>
          <MarketplacePreviewGallery previews={entry.previews} />
          <p className="text-xs text-muted-foreground">{t("plugins:previewInstallIndependent")}</p>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="flex gap-2">
            <Button
              type="button"
              variant={mode === "upload" ? "secondary" : "outline"}
              className="min-h-11 flex-1 cursor-pointer"
              onClick={() => updateMode("upload")}
            >
              {t("plugins:uploadBundle")}
            </Button>
            <Button
              type="button"
              variant={mode === "url" ? "secondary" : "outline"}
              className="min-h-11 flex-1 cursor-pointer"
              onClick={() => updateMode("url")}
            >
              {t("plugins:directLink")}
            </Button>
          </div>
          {mode === "upload" ? (
            <label className="block space-y-2 text-sm font-medium" htmlFor="canvas-install-file">
              {t("plugins:chooseBundle")}
              <Input
                id="canvas-install-file"
                type="file"
                accept={CANVAS_BUNDLE_ACCEPT}
                onChange={(event) => updateFile(event.target.files?.[0] ?? null)}
                className="min-h-11 cursor-pointer"
              />
            </label>
          ) : (
            <label className="block space-y-2 text-sm font-medium" htmlFor="canvas-install-url">
              {t("plugins:directLink")}
              <Input
                id="canvas-install-url"
                value={url}
                onChange={(event) => updateUrl(event.target.value)}
                placeholder={t("plugins:bundleUrlPlaceholder")}
                className="min-h-11"
                inputMode="url"
              />
            </label>
          )}
        </div>
      )}

      {Boolean(install.error) && (
        <p role="alert" className="text-sm text-destructive">
          {t("plugins:installFailed")}
        </p>
      )}
      {install.loading && !install.review && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("plugins:inspectingBundle")}
        </p>
      )}
      {install.review && !install.result && (
        <InstallReviewCard review={install.review} workspaceName={workspaceName} />
      )}
      {install.result && (
        <div
          className="space-y-3 rounded-lg border border-emerald-500/40 bg-emerald-500/10 p-4"
          role="status"
        >
          <p className="font-medium">{t("plugins:installedCanvas")}</p>
          <a
            className="text-sm underline"
            href={canvasHref(install.result.canvas.id)}
            onClick={() => onOpenChange(false)}
          >
            {t("plugins:openCanvas")}
          </a>
        </div>
      )}
    </div>
  );
  const footer = (
    <div className="flex shrink-0 flex-col-reverse gap-2 border-t px-4 py-3 md:flex-row md:justify-end">
      <Button type="button" variant="outline" className="min-h-11 cursor-pointer" onClick={close}>
        {t("common:cancel")}
      </Button>
      {!install.result &&
        (install.review ? (
          <Button
            type="button"
            className="min-h-11 cursor-pointer"
            disabled={install.loading}
            onClick={() => void install.confirm()}
          >
            {install.loading ? t("plugins:installingCanvas") : t("plugins:confirmInstall")}
          </Button>
        ) : (
          <Button
            type="button"
            className="min-h-11 cursor-pointer"
            disabled={install.loading || (!entry && (mode === "upload" ? !file : !url.trim()))}
            onClick={() => void inspect()}
          >
            {install.loading ? t("plugins:inspectingBundle") : t("plugins:reviewBundle")}
          </Button>
        ))}
    </div>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden">
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t("plugins:installCanvasTitle")}</DrawerTitle>
            <DrawerDescription>{t("plugins:installCanvasDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          <DrawerFooter className="shrink-0 p-0">{footer}</DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3 text-left">
          <DialogTitle>{t("plugins:installCanvasTitle")}</DialogTitle>
          <DialogDescription>{t("plugins:installCanvasDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        <DialogFooter className="p-0">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InstallReviewCard({
  review,
  workspaceName,
}: {
  review: NonNullable<ReturnType<typeof useCanvasInstall>["review"]>;
  workspaceName?: string;
}) {
  const { t } = useTranslation();
  const metadata = review.metadata;
  const groups = [
    [t("canvases:permissionReads"), review.permissions.reads ?? []],
    [t("canvases:permissionWrites"), review.permissions.writes ?? []],
    [t("canvases:permissionEvents"), review.permissions.events ?? []],
    [t("canvases:permissionExternalOrigins"), review.permissions.external_origins ?? []],
  ] as const;
  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 p-4"
      data-testid="canvas-install-review"
    >
      <div>
        <p className="font-medium">{t("plugins:packageVerified")}</p>
        <p className="text-sm text-muted-foreground">
          {metadata.display_name} · v{metadata.version}
        </p>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-muted-foreground">{t("plugins:packageId")}</dt>
        <dd className="break-all font-mono">{metadata.package_id}</dd>
        <dt className="text-muted-foreground">{t("plugins:packageDigest")}</dt>
        <dd className="break-all font-mono">{review.sha256}</dd>
        {review.archive_sha256 && (
          <>
            <dt className="text-muted-foreground">{t("plugins:archiveDigest")}</dt>
            <dd className="break-all font-mono">{review.archive_sha256}</dd>
          </>
        )}
        <dt className="text-muted-foreground">{t("plugins:source")}</dt>
        <dd>{review.origin_kind}</dd>
        <dt className="text-muted-foreground">{t("plugins:destination")}</dt>
        <dd>
          {workspaceName ?? t("plugins:workspace")} ({review.workspace_id})
        </dd>
        <dt className="text-muted-foreground">{t("canvases:description")}</dt>
        <dd>{metadata.description}</dd>
        <dt className="text-muted-foreground">{t("canvases:author")}</dt>
        <dd>{metadata.author}</dd>
        <dt className="text-muted-foreground">{t("canvases:license")}</dt>
        <dd>{metadata.license}</dd>
        <dt className="text-muted-foreground">{t("canvases:sourceMode")}</dt>
        <dd>{metadata.source_mode}</dd>
        <dt className="text-muted-foreground">{t("canvases:minKandevVersion")}</dt>
        <dd>{metadata.min_kandev_version}</dd>
        {metadata.repo_url && (
          <>
            <dt className="text-muted-foreground">{t("canvases:repositoryUrl")}</dt>
            <dd className="break-all">{metadata.repo_url}</dd>
          </>
        )}
      </dl>
      <div className="space-y-2">
        <h4 className="text-sm font-medium">{t("plugins:packagePermissions")}</h4>
        <div className="grid gap-2 sm:grid-cols-2">
          {groups.map(([label, values]) => (
            <div key={label} className="text-xs">
              <p className="text-muted-foreground">{label}</p>
              <p>{values.length ? values.join(", ") : t("plugins:none")}</p>
            </div>
          ))}
        </div>
        <div className="text-xs">
          <p className="text-muted-foreground">{t("canvases:sharedState")}</p>
          <p>{review.permissions.shared_state ? t("canvases:yes") : t("plugins:none")}</p>
        </div>
      </div>
    </section>
  );
}
