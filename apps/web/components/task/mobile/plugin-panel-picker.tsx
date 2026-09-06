"use client";

import { useTranslation } from "react-i18next";
import { IconHistory, IconLayoutGrid } from "@tabler/icons-react";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { pluginPanelId } from "@/lib/state/layout-manager/plugin-panels";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { MobilePickerSheet } from "./mobile-picker-sheet";

type PluginPanelPickerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (panel: MobileSessionPanel) => void;
  showPromptHistory?: boolean;
  taskCanvases?: Canvas[];
  onOpenCanvas?: (canvasId: string) => void;
};

/** One grouped, scrollable phone picker for all mobile-enabled plugin panels. */
export function PluginPanelPicker({
  open,
  onOpenChange,
  onSelect,
  showPromptHistory = false,
  taskCanvases = [],
  onOpenCanvas,
}: PluginPanelPickerProps) {
  const { t } = useTranslation();
  usePluginRegistry();
  const registrations = pluginRegistry
    .getTaskPanels()
    .filter((registration) => registration.mobileEnabled);

  if (!open) return null;

  return (
    <MobilePickerSheet open={open} onOpenChange={onOpenChange} title={t("common:panels")}>
      <div className="space-y-1" data-testid="mobile-plugin-panel-options">
        {taskCanvases.map((canvas) => (
          <button
            key={canvas.id}
            type="button"
            data-testid={`mobile-canvas-option-${canvas.id}`}
            className="flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => {
              onOpenCanvas?.(canvas.id);
              onOpenChange(false);
            }}
          >
            <IconLayoutGrid className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="min-w-0 truncate">{canvas.title}</span>
          </button>
        ))}
        {showPromptHistory && (
          <button
            type="button"
            data-testid="mobile-prompt-history-option"
            className="flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => {
              onSelect("prompt-history");
              onOpenChange(false);
            }}
          >
            <IconHistory className="h-5 w-5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 truncate">{t("task:promptHistory")}</span>
          </button>
        )}
        {registrations.map((registration) => {
          const panelId = pluginPanelId(registration.pluginId, registration.id);
          const Icon = resolvePluginIcon(registration.icon);
          return (
            <button
              key={panelId}
              type="button"
              data-testid={`mobile-plugin-panel-option-${registration.pluginId}-${registration.id}`}
              data-panel-id={panelId}
              className="flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => {
                onSelect(panelId as MobileSessionPanel);
                onOpenChange(false);
              }}
            >
              <Icon className="h-5 w-5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 truncate">{registration.title}</span>
            </button>
          );
        })}
      </div>
    </MobilePickerSheet>
  );
}
