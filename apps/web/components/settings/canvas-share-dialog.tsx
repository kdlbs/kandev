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
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useCanvasShare } from "@/hooks/domains/canvas/use-canvas-share";
import { useCanvasExportDefaults } from "@/hooks/domains/canvas/use-canvas-export-defaults";
import {
  type DistributionMetadata,
  type ExportDefaults,
} from "@/lib/api/domains/canvas-distribution-api";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { formatNumber } from "@/lib/i18n/formats";
import { CanvasShareHelp } from "./canvas-share-help";

type ShareMetadata = Omit<DistributionMetadata, "source_mode"> & {
  source_mode: "" | "static" | "project";
};

// eslint-disable-next-line complexity -- Seeding each editable distribution field keeps the form independent from API shape changes.
function seedShareMetadata(canvas: Canvas | null): ShareMetadata {
  const release = canvas?.active_release;
  const sourceMode =
    release?.source_mode === "project" || release?.source_mode === "static"
      ? release.source_mode
      : "";
  return {
    package_id: release?.package_id ?? "",
    version: release?.version ?? "",
    display_name: release?.display_name ?? canvas?.title ?? "",
    description: release?.description ?? "",
    author: release?.author ?? "",
    license: release?.license ?? "",
    source_mode: sourceMode,
    min_kandev_version: release?.min_kandev_version ?? "",
    repo_url: release?.repo_url ?? "",
  };
}

// eslint-disable-next-line max-lines-per-function -- The responsive share dialog owns one cohesive metadata and review flow.
export function CanvasShareDialog({
  canvas,
  open,
  onOpenChange,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const share = useCanvasShare(canvas);
  const { reset, cancel, invalidate } = share;
  const [metadata, setMetadata] = useState<ShareMetadata>(() => seedShareMetadata(canvas));
  const {
    defaults,
    loading: defaultsLoading,
    error: defaultsError,
    retry,
  } = useCanvasExportDefaults(
    canvas?.id,
    canvas?.active_release_id,
    open && canvas?.active_release_status === "valid",
  );
  const [detailsOpen, setDetailsOpen] = useState(false);

  useEffect(() => {
    if (open) {
      invalidate();
      setMetadata(seedShareMetadata(canvas));
      setDetailsOpen(false);
    } else {
      reset();
    }
  }, [canvas?.active_release_id, canvas?.id, open, reset, invalidate]);

  useEffect(() => {
    if (!defaults) return;
    const sourceMode = defaults.metadata.source_mode;
    setMetadata({
      ...seedShareMetadata(null),
      ...defaults.metadata,
      source_mode: sourceMode === "project" || sourceMode === "static" ? sourceMode : "",
    });
  }, [defaults]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) void cancel();
    onOpenChange(nextOpen);
  };
  const close = () => handleOpenChange(false);
  const updateMetadata = <K extends keyof ShareMetadata>(key: K, value: ShareMetadata[K]) => {
    invalidate();
    setMetadata((current) => ({ ...current, [key]: value }));
  };
  const prepare = () => {
    const required = [
      "package_id",
      "version",
      "display_name",
      "description",
      "author",
      "license",
      "source_mode",
      "min_kandev_version",
    ] as const;
    const missing = required.find((key) => !String(metadata[key] ?? "").trim());
    if (missing) {
      setDetailsOpen(true);
      const fieldId = defaults?.missing_required.includes(missing)
        ? `canvas-share-required-${missing}`
        : `canvas-share-${missing === "min_kandev_version" ? "min-version" : missing.replaceAll("_", "-")}`;
      requestAnimationFrame(() => document.getElementById(fieldId)?.focus());
      return;
    }
    void share
      .prepare({
        ...metadata,
        package_id: metadata.package_id?.trim(),
        version: metadata.version?.trim(),
        display_name: metadata.display_name?.trim(),
        description: metadata.description?.trim(),
        author: metadata.author?.trim(),
        license: metadata.license?.trim(),
        source_mode: metadata.source_mode || undefined,
        min_kandev_version: metadata.min_kandev_version?.trim(),
        repo_url: metadata.repo_url?.trim() || undefined,
      })
      .catch(() => undefined);
  };
  const body = (
    <CanvasShareBody
      canvas={canvas}
      defaults={defaults}
      defaultsLoading={defaultsLoading}
      defaultsError={defaultsError}
      onRetry={retry}
      metadata={metadata}
      onChange={updateMetadata}
      detailsOpen={detailsOpen}
      onDetailsOpenChange={setDetailsOpen}
      onPrepare={prepare}
      share={share}
    />
  );
  const footer = (
    <div className="flex shrink-0 flex-col-reverse gap-2 border-t px-4 py-3 md:flex-row md:justify-end">
      <Button
        variant="outline"
        className="min-h-11 cursor-pointer md:min-h-7 [@media(pointer:coarse)]:min-h-11"
        onClick={close}
      >
        {t("common:cancel")}
      </Button>
    </div>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden">
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t("canvases:shareCanvas")}</DrawerTitle>
            <DrawerDescription>{t("canvases:shareCanvasDescription")}</DrawerDescription>
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
          <DialogTitle>{t("canvases:shareCanvas")}</DialogTitle>
          <DialogDescription>{t("canvases:shareCanvasDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        <DialogFooter className="p-0">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type ShareBodyProps = {
  canvas: Canvas | null;
  defaults: ExportDefaults | null;
  defaultsLoading: boolean;
  defaultsError: boolean;
  onRetry: () => void;
  metadata: ShareMetadata;
  onChange: <K extends keyof ShareMetadata>(key: K, value: ShareMetadata[K]) => void;
  detailsOpen: boolean;
  onDetailsOpenChange: (open: boolean) => void;
  onPrepare: () => void;
  share: ReturnType<typeof useCanvasShare>;
};

function CanvasShareBody({
  canvas,
  defaults,
  defaultsLoading,
  defaultsError,
  onRetry,
  metadata,
  onChange,
  detailsOpen,
  onDetailsOpenChange,
  onPrepare,
  share,
}: ShareBodyProps) {
  const { t } = useTranslation();
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4">
      {!canvas?.active_release_id || canvas.active_release_status !== "valid" ? (
        <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
          {t("canvases:shareNoRelease")}
        </p>
      ) : (
        <>
          {defaultsLoading && (
            <p role="status" className="text-sm text-muted-foreground">
              {t("canvases:loadingShareDefaults")}
            </p>
          )}
          {defaultsError && (
            <div role="alert" className="space-y-2">
              <p className="text-sm text-destructive">{t("canvases:shareDefaultsFailed")}</p>
              <Button className="min-h-11" variant="outline" onClick={onRetry}>
                {t("canvases:retry")}
              </Button>
            </div>
          )}
          {defaults && (
            <>
              <p className="text-sm text-muted-foreground">
                {t("canvases:sharingRelease", {
                  name: metadata.display_name || canvas.title,
                  version: metadata.version,
                })}
              </p>
              {defaults.missing_required.length > 0 && (
                <div
                  className="rounded-lg border border-border/70 p-4"
                  data-testid="canvas-share-required-gaps"
                >
                  <p className="mb-3 text-sm font-medium">{t("canvases:shareRequiredDetails")}</p>
                  <ShareMissingFields
                    fields={defaults.missing_required}
                    metadata={metadata}
                    onChange={onChange}
                  />
                </div>
              )}
              <details
                open={detailsOpen}
                onToggle={(event) => onDetailsOpenChange(event.currentTarget.open)}
                className="rounded-lg border border-border/70"
                data-testid="canvas-share-package-details"
              >
                <summary className="min-h-11 cursor-pointer px-4 py-3 font-medium">
                  {t("canvases:packageDetails")}
                </summary>
                <ShareMetadataForm metadata={metadata} onChange={onChange} />
              </details>
            </>
          )}
          {!share.review && (
            <Button
              className="min-h-11 cursor-pointer md:min-h-7 [@media(pointer:coarse)]:min-h-11"
              disabled={share.loading || !defaults || defaultsLoading}
              onClick={onPrepare}
            >
              {t("canvases:prepareDownloads")}
            </Button>
          )}
          <span role="status" className="sr-only">
            {share.loading ? t("canvases:sharing") : ""}
          </span>
          {Boolean(share.error) && (
            <p role="alert" className="text-sm text-destructive">
              {t("canvases:shareFailed")}
            </p>
          )}
          {share.review && (
            <ExportReview
              review={share.review}
              loading={share.loading}
              onDownload={share.download}
            />
          )}
          <CanvasShareHelp review={share.review} />
        </>
      )}
    </div>
  );
}

function ExportReview({
  review,
  loading,
  onDownload,
}: {
  review: NonNullable<ReturnType<typeof useCanvasShare>["review"]>;
  loading: boolean;
  onDownload: (kind: "bundle" | "source") => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 p-4"
      data-testid="canvas-export-review"
    >
      <div>
        <h3 className="font-medium">{t("canvases:downloadsReady")}</h3>
        <p className="text-xs text-muted-foreground">
          {review.metadata.display_name ?? review.canvas_id} · v{review.metadata.version ?? ""}
        </p>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt className="text-muted-foreground">{t("canvases:packageId")}</dt>
        <dd className="break-all font-mono">{review.metadata.package_id ?? ""}</dd>
        <dt className="text-muted-foreground">{t("canvases:packageVersion")}</dt>
        <dd>{review.metadata.version ?? ""}</dd>
        <dt className="text-muted-foreground">{t("canvases:fileInventory")}</dt>
        <dd>
          <span>{t("canvases:fileCount", { count: review.files.length })}</span>
          <ul
            aria-label={t("canvases:fileInventory")}
            className="mt-1 max-h-36 space-y-1 overflow-y-auto rounded-md bg-muted/50 p-2 text-xs"
            data-testid="canvas-export-file-inventory"
          >
            {review.files.map((file) => (
              <li key={`${file.path}:${file.bytes}`} className="flex justify-between gap-3">
                <span className="min-w-0 break-all">{file.path}</span>
                <span className="shrink-0 text-muted-foreground">
                  {t("canvases:downloadSize", { size: formatNumber(file.bytes) })}
                </span>
              </li>
            ))}
          </ul>
        </dd>
        <dt className="text-muted-foreground">{t("canvases:bundleSize")}</dt>
        <dd>{t("canvases:downloadSize", { size: formatNumber(review.bundle_bytes) })}</dd>
        <dt className="text-muted-foreground">{t("canvases:sourceSize")}</dt>
        <dd>{t("canvases:downloadSize", { size: formatNumber(review.source_bytes) })}</dd>
      </dl>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          className="min-h-11 flex-1 cursor-pointer md:min-h-7 [@media(pointer:coarse)]:min-h-11"
          disabled={loading}
          onClick={() => void onDownload("bundle")}
        >
          {t("canvases:downloadBundle")}
        </Button>
        <Button
          variant="outline"
          className="min-h-11 flex-1 cursor-pointer md:min-h-7 [@media(pointer:coarse)]:min-h-11"
          disabled={loading}
          onClick={() => void onDownload("source")}
        >
          {t("canvases:downloadSource")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("canvases:privateContentReminder")}</p>
    </section>
  );
}

function ShareMissingFields({
  fields,
  metadata,
  onChange,
}: {
  fields: string[];
  metadata: ShareMetadata;
  onChange: <K extends keyof ShareMetadata>(key: K, value: ShareMetadata[K]) => void;
}) {
  const { t } = useTranslation();
  const labels: Record<string, string> = {
    package_id: "packageId",
    version: "packageVersion",
    display_name: "displayName",
    description: "description",
    author: "author",
    license: "license",
    source_mode: "sourceMode",
    min_kandev_version: "minKandevVersion",
  };
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {fields
        .filter((field) => field in labels)
        .map((field) => {
          const key = field as keyof ShareMetadata;
          const id = `canvas-share-required-${field}`;
          return (
            <div key={field} className="space-y-2">
              <Label htmlFor={id}>{t(`canvases:${labels[field]}`)}</Label>
              {field === "source_mode" ? (
                <Select
                  value={metadata.source_mode}
                  onValueChange={(value) =>
                    onChange("source_mode", value as ShareMetadata["source_mode"])
                  }
                >
                  <SelectTrigger id={id} className="min-h-11">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="static">{t("canvases:sourceModeStatic")}</SelectItem>
                    <SelectItem value="project">{t("canvases:sourceModeProject")}</SelectItem>
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id={id}
                  className="min-h-11"
                  value={String(metadata[key] ?? "")}
                  onChange={(event) => onChange(key, event.target.value)}
                />
              )}
            </div>
          );
        })}
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function -- The metadata fields stay together so every edit shares the same invalidation path.
function ShareMetadataForm({
  metadata,
  onChange,
}: {
  metadata: ShareMetadata;
  onChange: <K extends keyof ShareMetadata>(key: K, value: ShareMetadata[K]) => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="space-y-3 rounded-lg border border-border/70 p-4">
      <div>
        <h3 className="font-medium">{t("canvases:distributionMetadata")}</h3>
        <p className="text-xs text-muted-foreground">{t("canvases:distributionMetadataHelp")}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="canvas-share-package-id">{t("canvases:packageId")}</Label>
          <Input
            id="canvas-share-package-id"
            value={metadata.package_id ?? ""}
            onChange={(event) => onChange("package_id", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-version">{t("canvases:packageVersion")}</Label>
          <Input
            id="canvas-share-version"
            value={metadata.version ?? ""}
            onChange={(event) => onChange("version", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-display-name">{t("canvases:displayName")}</Label>
          <Input
            id="canvas-share-display-name"
            value={metadata.display_name ?? ""}
            onChange={(event) => onChange("display_name", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-author">{t("canvases:author")}</Label>
          <Input
            id="canvas-share-author"
            value={metadata.author ?? ""}
            onChange={(event) => onChange("author", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-license">{t("canvases:license")}</Label>
          <Input
            id="canvas-share-license"
            value={metadata.license ?? ""}
            onChange={(event) => onChange("license", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-min-version">{t("canvases:minKandevVersion")}</Label>
          <Input
            id="canvas-share-min-version"
            value={metadata.min_kandev_version ?? ""}
            onChange={(event) => onChange("min_kandev_version", event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-source-mode">{t("canvases:sourceMode")}</Label>
          <Select
            value={metadata.source_mode}
            onValueChange={(value) =>
              onChange("source_mode", value as ShareMetadata["source_mode"])
            }
          >
            <SelectTrigger id="canvas-share-source-mode">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="static">{t("canvases:sourceModeStatic")}</SelectItem>
              <SelectItem value="project">{t("canvases:sourceModeProject")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="canvas-share-repo-url">{t("canvases:repositoryUrl")}</Label>
          <Input
            id="canvas-share-repo-url"
            value={metadata.repo_url ?? ""}
            onChange={(event) => onChange("repo_url", event.target.value)}
            inputMode="url"
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="canvas-share-description">{t("canvases:description")}</Label>
        <Textarea
          id="canvas-share-description"
          value={metadata.description ?? ""}
          onChange={(event) => onChange("description", event.target.value)}
          rows={3}
        />
      </div>
    </section>
  );
}
