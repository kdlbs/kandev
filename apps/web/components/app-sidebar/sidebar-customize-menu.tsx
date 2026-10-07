"use client";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { SIDEBAR_LAYOUT_TAB_HREF } from "@/lib/settings-discovery/catalog/preferences";
import { useRouter } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuCheckboxItem,
  ContextMenuSeparator,
  ContextMenuLabel,
  ContextMenuItem,
  ContextMenuRadioGroup,
  ContextMenuRadioItem,
} from "@kandev/ui/context-menu";
import { moveSection, toggleNodeVisibility } from "@/lib/sidebar/layout-operations";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";
import { SidebarCustomizationFeedback } from "./sidebar-customization-feedback";
import { useSidebarCustomization } from "@/hooks/domains/sidebar/use-sidebar-customization";

export function SidebarCustomizeMenu({
  nodes,
  catalog,
  children,
}: {
  nodes: ProjectedSidebarNode[];
  catalog: ShortcutCatalogEntry[];
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const fast = useAppStore((s) => s.userSettings.sidebarFastActionsEnabled);
  const style = useAppStore((s) => s.userSettings.sidebarNewTaskStyle);
  const { mutate, preferences, status, enabled } = useSidebarCustomization(catalog);
  const [target, setTarget] = useState<string | null>(null);
  const targetIndex = nodes.findIndex((node) => node.id === target);
  const move = (delta: number) =>
    void mutate((layout) => {
      const index = layout.nodes.findIndex((node) => node.id === target);
      const neighbour = nodes[targetIndex + delta];
      return neighbour && index >= 0
        ? moveSection(
            layout,
            target!,
            layout.nodes.findIndex((node) => node.id === neighbour.id),
          )
        : layout;
    });
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild disabled={!enabled}>
        <div
          className="min-h-6 min-w-0"
          aria-label={t("settings:sidebarCustomize")}
          tabIndex={nodes.every((node) => !node.visible) ? 0 : undefined}
          data-testid="sidebar-customize-region"
          onContextMenu={(event) => {
            setTarget(
              (event.target as Element).closest<HTMLElement>("[data-sidebar-node-id]")?.dataset
                .sidebarNodeId ?? null,
            );
          }}
        >
          {children}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent
        className="w-64 max-h-[80dvh] overflow-y-auto"
        data-testid="sidebar-customize-menu"
      >
        <ContextMenuLabel>{t("settings:sidebarSettingsTitle")}</ContextMenuLabel>
        <ContextMenuSeparator />
        {nodes.map((node) => (
          <ContextMenuCheckboxItem
            key={node.id}
            checked={node.visible}
            disabled={status === "saving"}
            data-testid={`sidebar-visibility-${node.id}`}
            onCheckedChange={(visible) =>
              void mutate((layout) => toggleNodeVisibility(layout, node.id, visible))
            }
          >
            {node.label}
          </ContextMenuCheckboxItem>
        ))}
        <ContextMenuSeparator />
        <PresentationMenuOptions
          fast={fast}
          style={style}
          busy={status === "saving"}
          preferences={preferences}
        />
        {targetIndex >= 0 && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              disabled={targetIndex === 0 || status === "saving"}
              onSelect={() => move(-1)}
            >
              {t("settings:moveUp")}
            </ContextMenuItem>
            <ContextMenuItem
              disabled={targetIndex === nodes.length - 1 || status === "saving"}
              onSelect={() => move(1)}
            >
              {t("settings:moveDown")}
            </ContextMenuItem>
          </>
        )}
        <ContextMenuSeparator />
        <ContextMenuItem onSelect={() => router.push(SIDEBAR_LAYOUT_TAB_HREF)}>
          {t("settings:sidebarLayoutSettings")}
        </ContextMenuItem>
      </ContextMenuContent>
      <SidebarCustomizationFeedback status={status} />
    </ContextMenu>
  );
}

function PresentationMenuOptions({
  fast,
  style,
  busy,
  preferences,
}: {
  fast: boolean;
  style: "simple" | "compact";
  busy: boolean;
  preferences: ReturnType<typeof useSidebarCustomization>["preferences"];
}) {
  const { t } = useTranslation();
  return (
    <>
      <ContextMenuCheckboxItem
        checked={fast}
        disabled={busy}
        onCheckedChange={(value) => void preferences({ sidebar_fast_actions_enabled: value })}
      >
        {t("settings:sidebarFastActions")}
      </ContextMenuCheckboxItem>
      <ContextMenuRadioGroup
        value={style}
        onValueChange={(value) =>
          void preferences({ sidebar_new_task_style: value as "simple" | "compact" })
        }
      >
        <ContextMenuRadioItem value="simple" disabled={busy}>
          {t("settings:sidebarStyleSimple")}
        </ContextMenuRadioItem>
        <ContextMenuRadioItem value="compact" disabled={busy}>
          {t("settings:sidebarStyleCompact")}
        </ContextMenuRadioItem>
      </ContextMenuRadioGroup>
    </>
  );
}
