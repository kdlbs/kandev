"use client";

import { useState } from "react";
import { IconChevronLeft, IconChevronRight, IconMaximize, IconPhoto } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { useTranslation } from "react-i18next";
import type { MarketplacePreview } from "@/lib/types/plugins";

// eslint-disable-next-line max-lines-per-function -- The gallery keeps image, navigation, and enlargement state together.
export function MarketplacePreviewGallery({ previews }: { previews?: MarketplacePreview[] }) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(0);
  const [failed, setFailed] = useState<Record<number, boolean>>({});
  const [expanded, setExpanded] = useState(false);
  if (!previews?.length) return null;
  const selected = previews[Math.min(index, previews.length - 1)];
  const hasMany = previews.length > 1;
  const move = (delta: number) =>
    setIndex((value) => (value + delta + previews.length) % previews.length);
  return (
    <div className="space-y-2" aria-label={t("plugins:previewImages")}>
      <div className="relative overflow-hidden rounded-lg border border-border/70 bg-muted/30 aspect-video">
        {failed[index] ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 p-4 text-center text-xs text-muted-foreground">
            <IconPhoto className="h-6 w-6" />
            <span>{t("plugins:previewImageFailed")}</span>
            <Button
              variant="ghost"
              size="sm"
              className="cursor-pointer"
              onClick={() => setFailed((value) => ({ ...value, [index]: false }))}
            >
              {t("plugins:retry")}
            </Button>
          </div>
        ) : (
          <img
            src={selected.url}
            alt={selected.alt}
            className="h-full w-full object-cover"
            loading="lazy"
            referrerPolicy="no-referrer"
            onError={() => setFailed((value) => ({ ...value, [index]: true }))}
          />
        )}
        <Button
          variant="secondary"
          size="icon"
          className="absolute bottom-2 right-2 cursor-pointer"
          aria-label={t("plugins:enlargePreview")}
          onClick={() => setExpanded(true)}
        >
          <IconMaximize className="h-4 w-4" />
        </Button>
      </div>
      {hasMany && (
        <div className="flex items-center justify-between gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="cursor-pointer"
            aria-label={t("plugins:previousPreview")}
            onClick={() => move(-1)}
          >
            <IconChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-xs text-muted-foreground" aria-live="polite">
            {t("plugins:previewPosition", { current: index + 1, total: previews.length })}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="cursor-pointer"
            aria-label={t("plugins:nextPreview")}
            onClick={() => move(1)}
          >
            <IconChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}
      {hasMany && (
        <div className="flex gap-2 overflow-x-auto pb-1">
          {previews.map((preview, previewIndex) => (
            <button
              key={`${preview.url}-${previewIndex}`}
              type="button"
              className={`h-12 w-16 shrink-0 overflow-hidden rounded border cursor-pointer ${previewIndex === index ? "border-primary ring-1 ring-primary" : "border-border/70"}`}
              aria-label={t("plugins:selectPreview", { position: previewIndex + 1 })}
              onClick={() => setIndex(previewIndex)}
            >
              <img
                src={preview.url}
                alt=""
                className="h-full w-full object-cover"
                loading="lazy"
                referrerPolicy="no-referrer"
              />
            </button>
          ))}
        </div>
      )}
      <Dialog open={expanded} onOpenChange={setExpanded}>
        <DialogContent className="max-w-5xl">
          <DialogTitle>{t("plugins:previewImages")}</DialogTitle>
          <img
            src={selected.url}
            alt={selected.alt}
            className="max-h-[75vh] w-full object-contain"
            referrerPolicy="no-referrer"
          />
        </DialogContent>
      </Dialog>
    </div>
  );
}
