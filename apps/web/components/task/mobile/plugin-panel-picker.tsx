"use client";

import { useTranslation } from "react-i18next";
import { useRef } from "react";
import { IconLayoutGrid, IconNetwork, IconChevronRight } from "@tabler/icons-react";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { pluginPanelId } from "@/lib/state/layout-manager/plugin-panels";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { registrationIsVisible } from "../plugin-task-panel";
import { resolveTaskPanelTitle } from "@/lib/state/layout-manager/plugin-panels";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import {
  type PortForwardingVisibility,
  useOptionalPortForwardingVisibility,
} from "../port-forwarding-visibility-provider";

type PluginPanelPickerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (panel: MobileSessionPanel) => void;
  taskCanvases?: Canvas[];
  onOpenCanvas?: (canvasId: string) => void;
  taskId?: string | null;
  sessionId?: string | null;
  sessionKind?: "managed" | "passthrough" | null;
};

/** One grouped, scrollable phone picker for all mobile-enabled plugin panels. */
export function PluginPanelPicker({
  open,
  onOpenChange,
  onSelect,
  taskCanvases = [],
  onOpenCanvas,
  taskId = null,
  sessionId = null,
  sessionKind = null,
}: PluginPanelPickerProps) {
  const { t } = useTranslation();
  const portForwarding = useOptionalPortForwardingVisibility();
  const openingPorts = useRef(false);
  usePluginRegistry();
  const registrations = taskId
    ? pluginRegistry
        .getTaskPanels()
        .filter((registration) => registration.mobileEnabled)
        .filter((registration) =>
          registrationIsVisible(registration, {
            taskId,
            sessionId,
            sessionKind,
            presentation: "mobile",
          }),
        )
    : [];

  if (!open) return null;

  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t("common:panels")}
      onCloseAutoFocus={(event) => {
        if (openingPorts.current) event.preventDefault();
        openingPorts.current = false;
      }}
    >
      <div className="space-y-1" data-testid="mobile-plugin-panel-options">
        {taskId && portForwarding && (
          <PortForwardingPickerAction
            visibility={portForwarding}
            onSelect={() => {
              openingPorts.current = true;
              onOpenChange(false);
              portForwarding.setDialogOpen(true);
            }}
          />
        )}
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
              <span className="min-w-0 truncate">{resolveTaskPanelTitle(registration)}</span>
            </button>
          );
        })}
      </div>
    </MobilePickerSheet>
  );
}

function PortForwardingPickerAction({
  visibility,
  onSelect,
}: {
  visibility: PortForwardingVisibility;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const disabled = !visibility.canToggle || visibility.isUpdating;
  return (
    <>
      <h3 className="px-3 py-2 text-xs font-medium text-muted-foreground">{t("task:taskTools")}</h3>
      <button
        type="button"
        data-testid="mobile-port-forwarding-open"
        className="flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
        aria-label={t("task:portForwarding")}
        aria-describedby={disabled ? "mobile-port-forwarding-unavailable" : undefined}
        disabled={disabled}
        onClick={onSelect}
      >
        <IconNetwork className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="min-w-0 flex-1">{t("task:portForwarding")}</span>
        <IconChevronRight className="h-4 w-4 shrink-0" aria-hidden="true" />
      </button>
      {disabled && (
        <p
          id="mobile-port-forwarding-unavailable"
          className="px-3 pb-2 text-xs text-muted-foreground"
        >
          {visibility.isUpdating ? t("task:saving") : t("task:portForwardingUnavailable")}
        </p>
      )}
    </>
  );
}
