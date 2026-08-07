"use client";

import { IconPlus, IconMinus, IconArrowsMaximize, IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

type OrgZoomControlsProps = {
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
  onExport?: () => void;
};

export function OrgZoomControls({ onZoomIn, onZoomOut, onFit, onExport }: OrgZoomControlsProps) {
  const { t } = useTranslation();
  return (
    <div className="absolute top-4 right-4 z-10 flex flex-col gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8 cursor-pointer"
            onClick={onZoomIn}
          >
            <IconPlus className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">{t("office:zoomIn")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8 cursor-pointer"
            onClick={onZoomOut}
          >
            <IconMinus className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">{t("office:zoomOut")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline" size="icon" className="h-8 w-8 cursor-pointer" onClick={onFit}>
            <IconArrowsMaximize className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">{t("office:fitToScreen")}</TooltipContent>
      </Tooltip>
      {onExport && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={onExport}
            >
              <IconDownload className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">{t("office:exportSvg")}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
