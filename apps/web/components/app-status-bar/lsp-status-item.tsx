"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { SurfaceAction } from "@/components/actions/surface-action";
import {
  LspStatusIcon,
  LspStatusPopoverContent,
  useLspLiveNow,
} from "@/components/editors/lsp-status-button";
import { useLspStatus } from "@/hooks/use-lsp";
import { toLspLanguage } from "@/lib/lsp/lsp-client-manager";
import { getLspCompactSummary } from "@/lib/lsp/lsp-progress-view";

type LspStatusItemProps = {
  sessionId: string;
  monacoLanguage: string;
};

function languageLabel(language: string): string {
  if (language === "typescript") return "TypeScript";
  return language.charAt(0).toUpperCase() + language.slice(1);
}

export function LspStatusItem({ sessionId, monacoLanguage }: LspStatusItemProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const lspLanguage = toLspLanguage(monacoLanguage);
  const { status, progress, toggle } = useLspStatus(sessionId, lspLanguage);
  const tracked = progress.active[0]?.startedAt ?? progress.initializingSince;
  const now = useLspLiveNow(tracked !== null);
  if (!lspLanguage) return null;

  const language = languageLabel(lspLanguage);
  const summary = getLspCompactSummary(status, progress, now);
  const description = t("lsp:languageServerDescription", { language, summary });

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip open={open ? false : undefined}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <SurfaceAction
              surface="status-bar"
              presentation="desktop"
              label={t("lsp:openStatus", { description })}
              icon={<LspStatusIcon status={status} progress={progress} />}
              aria-haspopup="dialog"
              aria-expanded={open}
              data-testid="app-status-lsp"
              data-lsp-state={status.state}
              data-lsp-language={lspLanguage}
            >
              <span className="shrink-0 font-semibold text-foreground">{language}</span>
              <span aria-hidden className="text-muted-foreground">
                ·
              </span>
              <span className="truncate text-muted-foreground tabular-nums">{summary}</span>
            </SurfaceAction>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{description}</TooltipContent>
      </Tooltip>
      <LspStatusPopoverContent
        status={status}
        progress={progress}
        lspLanguage={lspLanguage}
        onToggle={toggle}
        align="start"
        side="top"
      />
    </Popover>
  );
}
