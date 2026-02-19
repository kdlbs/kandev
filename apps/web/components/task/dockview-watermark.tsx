"use client";

import { useCallback, useMemo } from "react";
import type { IWatermarkPanelProps } from "dockview-react";
import {
  IconMessage,
  IconFileText,
  IconTerminal2,
  IconDeviceDesktop,
  IconGitBranch,
  IconFolder,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { PANEL_REGISTRY } from "@/lib/state/layout-manager";
import { useAppStore } from "@/components/state-provider";
import { createUserShell } from "@/lib/api/domains/user-shell-api";

type PanelOption = {
  id: string;
  label: string;
  icon: React.ElementType;
  /** Whether only one instance is allowed (focus existing instead of adding). */
  singleton?: boolean;
};

const PANEL_OPTIONS: PanelOption[] = [
  { id: "chat", label: "Agent", icon: IconMessage, singleton: true },
  { id: "plan", label: "Plan", icon: IconFileText },
  { id: "terminal", label: "Terminal", icon: IconTerminal2 },
  { id: "browser", label: "Browser", icon: IconDeviceDesktop },
  { id: "changes", label: "Changes", icon: IconGitBranch, singleton: true },
  { id: "files", label: "Files", icon: IconFolder, singleton: true },
];

export function DockviewWatermark({ containerApi, group }: IWatermarkPanelProps) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);

  const handleAdd = useCallback(
    async (option: PanelOption) => {
      const groupId = group?.id;

      if (option.id === "terminal") {
        let terminalId = `terminal-${Date.now()}`;
        if (activeSessionId) {
          try {
            const result = await createUserShell(activeSessionId);
            terminalId = result.terminalId;
          } catch {
            // Fall back to default terminal ID
          }
        }
        containerApi.addPanel({
          id: terminalId,
          component: "terminal",
          title: "Terminal",
          params: { terminalId },
          ...(groupId ? { position: { referenceGroup: groupId } } : {}),
        });
        return;
      }

      if (option.id === "browser") {
        const browserId = `browser:${Date.now()}`;
        containerApi.addPanel({
          id: browserId,
          component: "browser",
          title: "Browser",
          params: { url: "" },
          ...(groupId ? { position: { referenceGroup: groupId } } : {}),
        });
        return;
      }

      // Singleton panels — focus if exists, otherwise add to this group
      if (option.singleton) {
        const existing = containerApi.getPanel(option.id);
        if (existing) {
          existing.api.setActive();
          return;
        }
      }

      const config = PANEL_REGISTRY[option.id];
      if (!config) return;

      containerApi.addPanel({
        id: option.id,
        component: config.component,
        title: config.title,
        ...(config.tabComponent ? { tabComponent: config.tabComponent } : {}),
        ...(groupId ? { position: { referenceGroup: groupId } } : {}),
      });
    },
    [containerApi, group, activeSessionId],
  );

  // Check which singletons already exist so we can hide them
  const existingPanels = useMemo(() => {
    const ids = new Set<string>();
    try {
      for (const panel of containerApi.panels) {
        ids.add(panel.id);
      }
    } catch {
      // API may not be ready
    }
    return ids;
  }, [containerApi]);

  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const isSidebarGroup = group?.id === sidebarGroupId;
  if (isSidebarGroup) return null;

  return (
    <div className="flex items-center justify-center h-full w-full">
      <div className="flex flex-wrap gap-2 justify-center max-w-xs">
        {PANEL_OPTIONS.map((option) => {
          if (option.singleton && existingPanels.has(option.id)) return null;
          const Icon = option.icon;
          return (
            <Button
              key={option.id}
              variant="outline"
              size="sm"
              className="cursor-pointer gap-1.5 text-muted-foreground hover:text-foreground"
              onClick={() => handleAdd(option)}
            >
              <Icon className="h-3.5 w-3.5" />
              {option.label}
            </Button>
          );
        })}
      </div>
    </div>
  );
}
