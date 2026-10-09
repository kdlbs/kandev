"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowUp, IconArrowDown, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Switch } from "@kandev/ui/switch";
import {
  DrawerNested,
  DrawerTrigger,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerClose,
} from "@kandev/ui/drawer";
import { SIDEBAR_LAYOUT_TAB_HREF } from "@/lib/settings-discovery/catalog/preferences";
import { useRouter } from "@/lib/routing/client-router";
import { useAppStore } from "@/components/state-provider";
import { useSidebarCustomization } from "@/hooks/domains/sidebar/use-sidebar-customization";
import { toggleNodeVisibility, moveSection } from "@/lib/sidebar/layout-operations";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";

export function MobileSidebarCustomization({
  nodes,
  catalog,
  onNavigate,
}: {
  nodes: ProjectedSidebarNode[];
  catalog: ShortcutCatalogEntry[];
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const { mutate, preferences, enabled, status } = useSidebarCustomization(catalog);
  const fast = useAppStore((s) => s.userSettings.sidebarFastActionsEnabled);
  const style = useAppStore((s) => s.userSettings.sidebarNewTaskStyle);
  const busy = status === "saving";
  return (
    <DrawerNested open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <Button
          variant="outline"
          className="min-h-11 w-full"
          disabled={!enabled}
          data-testid="mobile-sidebar-customize"
        >
          {t("settings:sidebarCustomize")}
        </Button>
      </DrawerTrigger>
      <DrawerContent
        aria-describedby={undefined}
        data-testid="mobile-sidebar-customization"
        className="max-h-[calc(100dvh-24px-env(safe-area-inset-bottom,0px))]"
      >
        <DrawerHeader className="flex-row items-center justify-between border-b text-left">
          <DrawerTitle>{t("settings:sidebarCustomize")}</DrawerTitle>
          <DrawerClose asChild>
            <Button variant="ghost" size="icon" className="size-11" aria-label={t("common:close")}>
              <IconX className="size-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div
          className="min-h-0 overflow-y-auto px-4 pb-[max(16px,env(safe-area-inset-bottom))]"
          data-vaul-no-drag
        >
          <CustomizationRows nodes={nodes} busy={busy} mutate={mutate} />
          <CustomizationPreferences
            fast={fast}
            style={style}
            busy={busy}
            preferences={preferences}
          />
          {(status === "error" || status === "conflict") && (
            <p role="alert" className="text-sm text-destructive">
              {t(
                status === "conflict"
                  ? "settings:sidebarLayoutConflict"
                  : "settings:sidebarSaveError",
              )}
            </p>
          )}
          <Button
            variant="outline"
            className="min-h-11 w-full"
            onClick={() => {
              setOpen(false);
              onNavigate();
              router.push(SIDEBAR_LAYOUT_TAB_HREF);
            }}
          >
            {t("settings:sidebarLayoutSettings")}
          </Button>
        </div>
      </DrawerContent>
    </DrawerNested>
  );
}

function CustomizationRows({
  nodes,
  busy,
  mutate,
}: {
  nodes: ProjectedSidebarNode[];
  busy: boolean;
  mutate: ReturnType<typeof useSidebarCustomization>["mutate"];
}) {
  const { t } = useTranslation();
  return (
    <>
      {nodes.map((node, index) => (
        <div key={node.id} className="flex min-h-11 items-center gap-2 border-b py-1">
          <label className="flex min-h-11 min-w-0 flex-1 items-center gap-3">
            <Checkbox
              checked={node.visible}
              disabled={busy}
              data-testid={`mobile-sidebar-visibility-${node.id}`}
              onCheckedChange={(checked) =>
                void mutate((layout) => toggleNodeVisibility(layout, node.id, checked === true))
              }
            />
            <span className="truncate text-sm">{node.label}</span>
          </label>
          {[-1, 1].map((delta) => (
            <Button
              key={delta}
              variant="ghost"
              size="icon"
              className="size-11"
              disabled={busy || !nodes[index + delta]}
              aria-label={t(
                delta < 0 ? "settings:sidebarMoveUpEntry" : "settings:sidebarMoveDownEntry",
                { name: node.label },
              )}
              onClick={() =>
                void mutate((layout) =>
                  moveSection(
                    layout,
                    node.id,
                    layout.nodes.findIndex((item) => item.id === nodes[index + delta].id),
                  ),
                )
              }
            >
              {delta < 0 ? (
                <IconArrowUp className="size-4" />
              ) : (
                <IconArrowDown className="size-4" />
              )}
            </Button>
          ))}
        </div>
      ))}
    </>
  );
}

function CustomizationPreferences({
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
      <label className="flex min-h-11 items-center justify-between gap-3 py-2 text-sm">
        {t("settings:sidebarFastActions")}
        <Switch
          checked={fast}
          disabled={busy}
          onCheckedChange={(value) => void preferences({ sidebar_fast_actions_enabled: value })}
        />
      </label>
      <fieldset className="space-y-1 border-t py-2">
        <legend className="pt-2 text-sm">{t("settings:sidebarNewTaskStyle")}</legend>
        {(["simple", "compact"] as const).map((value) => (
          <label key={value} className="flex min-h-11 items-center gap-3 text-sm">
            <input
              type="radio"
              name="mobile-sidebar-style"
              value={value}
              checked={style === value}
              disabled={busy}
              onChange={() => void preferences({ sidebar_new_task_style: value })}
            />
            {t(value === "simple" ? "settings:sidebarStyleSimple" : "settings:sidebarStyleCompact")}
          </label>
        ))}
      </fieldset>
    </>
  );
}
