"use client";

import { useCallback, useState } from "react";
import { IconBrowser, IconCheck, IconCopy, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useTranslation } from "react-i18next";

export function PortUrlActions({
  url,
  onOpenBrowserPanel,
  browserActionTestId,
}: {
  url: string;
  onOpenBrowserPanel?: (url: string) => void;
  browserActionTestId?: string;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(async () => {
    if (await copyToClipboard(url)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }, [url]);

  return (
    <div className="flex items-center gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            aria-label={t("task:copyUrl")}
            className="cursor-pointer min-h-11 min-w-11 p-0 sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0"
            onClick={handleCopy}
          >
            {copied ? (
              <IconCheck className="h-3.5 w-3.5 text-green-500" />
            ) : (
              <IconCopy className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:copyUrl")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            aria-label={t("task:openInNewTab")}
            className="cursor-pointer min-h-11 min-w-11 p-0 sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0"
            asChild
          >
            <a href={url} target="_blank" rel="noopener noreferrer">
              <IconExternalLink className="h-3.5 w-3.5" />
            </a>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:openInNewTab")}</TooltipContent>
      </Tooltip>
      {onOpenBrowserPanel && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              aria-label={t("task:openInBrowserPanel")}
              data-testid={browserActionTestId}
              className="cursor-pointer min-h-11 min-w-11 p-0 sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0"
              onClick={() => onOpenBrowserPanel(url)}
            >
              <IconBrowser className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("task:openInBrowserPanel")}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
