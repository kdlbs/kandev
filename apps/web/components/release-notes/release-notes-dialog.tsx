"use client";

import ReactMarkdown from "react-markdown";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@kandev/ui/dialog";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Badge } from "@kandev/ui/badge";
import { Separator } from "@kandev/ui/separator";
import { IconExternalLink } from "@tabler/icons-react";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";
import { getReleaseUrl } from "@/lib/release-notes";
import type { ChangelogEntry } from "@/lib/changelog";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

type ReleaseNotesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entries: ChangelogEntry[];
  latestVersion: string;
};

function buildDescription(t: TFunction, entries: ChangelogEntry[]): string {
  if (entries.length === 1) {
    return entries[0].date
      ? t("common:releasedOn", { date: entries[0].date })
      : t("common:latestReleaseNotes");
  }
  return t("common:newReleases", { count: entries.length });
}

export function ReleaseNotesDialog({
  open,
  onOpenChange,
  entries,
  latestVersion,
}: ReleaseNotesDialogProps) {
  const { t } = useTranslation();
  const releaseUrl = getReleaseUrl(latestVersion);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {t("common:whatSNew")}
            <Badge variant="secondary">v{latestVersion}</Badge>
          </DialogTitle>
          <DialogDescription>{buildDescription(t, entries)}</DialogDescription>
        </DialogHeader>
        <ScrollArea className="max-h-[60vh] pr-4">
          <div className="space-y-4">
            {entries.map((entry, index) => (
              <div key={entry.version}>
                {entries.length > 1 && (
                  <div className="flex items-center gap-2 mb-2">
                    <Badge variant="outline">v{entry.version}</Badge>
                    {entry.date && (
                      <span className="text-xs text-muted-foreground">{entry.date}</span>
                    )}
                  </div>
                )}
                <div className="markdown-body text-sm">
                  <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
                    {entry.notes}
                  </ReactMarkdown>
                </div>
                {index < entries.length - 1 && <Separator className="mt-4" />}
              </div>
            ))}
          </div>
        </ScrollArea>
        <div className="pt-2 border-t border-border">
          <a
            href={releaseUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            {t("common:viewFullReleaseOnGithub")}
            <IconExternalLink className="h-3 w-3" />
          </a>
        </div>
      </DialogContent>
    </Dialog>
  );
}
