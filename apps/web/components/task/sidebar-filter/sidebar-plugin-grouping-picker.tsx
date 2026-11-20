"use client";

import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconLayoutList, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerNested,
  DrawerTrigger,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerClose,
} from "@kandev/ui/drawer";
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent } from "@kandev/ui/dropdown-menu";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useSidebarGroupings, type SidebarGroupingChoices } from "@/hooks/use-sidebar-groupings";
import { SidebarPluginGroupingMenu } from "./sidebar-plugin-grouping-menu";
import { useSidebarViewTransition } from "@/hooks/domains/sidebar/use-sidebar-view-transition";

type Surface = "inline" | "sheet" | "drawer";

export function SidebarPluginGroupingPicker({
  parentSurface = "sheet",
}: {
  parentSurface?: Surface;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const coarse = useTouchDrawer();
  const touch = isMobile || coarse;
  const choices = useSidebarGroupings();
  const { interactionProps } = useSidebarViewTransition();
  const [menuOpen, setMenuOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    // The parent drawer restores its geometry through the close primitive.
    closeRef.current?.click();
    setMenuOpen(false);
  }, [choices.workspaceId, choices.groupings.length, touch]);
  if (!choices.groupings.length && !drawerOpen) return null;
  const trigger = (
    <Button
      {...interactionProps}
      type="button"
      variant="ghost"
      size="icon"
      className="size-7 shrink-0 cursor-pointer [@media(pointer:coarse)]:size-11 [@media(max-width:767px)]:size-11"
      aria-label={t("sidebar:pluginGroupingSection")}
    >
      <IconLayoutList className="size-4" aria-hidden="true" />
    </Button>
  );
  if (touch || drawerOpen)
    return (
      <GroupingChoiceDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        nested={parentSurface === "drawer" || (parentSurface === "inline" && isMobile)}
        trigger={trigger}
        choices={choices}
        closeRef={closeRef}
      />
    );
  return (
    <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent className="w-64 max-w-[calc(100vw-2rem)] overflow-y-auto">
        <SidebarPluginGroupingMenu separator={false} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function GroupingChoiceDrawer({
  open,
  onOpenChange,
  nested,
  trigger,
  choices,
  closeRef,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  nested: boolean;
  trigger: ReactNode;
  choices: SidebarGroupingChoices;
  closeRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const Root = nested ? DrawerNested : Drawer;
  return (
    <Root open={open} onOpenChange={onOpenChange}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent
        aria-describedby={undefined}
        data-testid="sidebar-grouping-chooser"
        className="!max-h-[calc(100dvh-env(safe-area-inset-top)-env(safe-area-inset-bottom)-1rem)] pb-[max(.5rem,env(safe-area-inset-bottom))]"
      >
        <div className="flex min-h-0 flex-col overflow-hidden rounded-xl bg-background">
          <DrawerHeader className="!flex-row shrink-0 items-center justify-between text-left">
            <DrawerTitle>{t("sidebar:pluginGroupingSection")}</DrawerTitle>
            <DrawerClose asChild>
              <Button
                ref={closeRef}
                variant="ghost"
                size="icon"
                className="size-11 shrink-0"
                aria-label={t("common:close")}
              >
                <IconX className="size-4" />
              </Button>
            </DrawerClose>
          </DrawerHeader>
          <div
            className="min-h-0 overflow-y-auto overscroll-contain px-2 pb-2"
            data-testid="sidebar-grouping-choices"
          >
            {choices.groupings.map((choice) => (
              <DrawerClose key={choice.reference} asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={Boolean(choice.disabledReason)}
                  data-testid="sidebar-plugin-grouping"
                  data-grouping-reference={choice.reference}
                  className="min-h-[44px] h-auto w-full flex-col items-start justify-center whitespace-normal px-3 py-2 text-left"
                  onClick={(event) => {
                    const request = choices.selectGrouping(choice);
                    if (request === undefined) event.preventDefault();
                    else void request;
                  }}
                >
                  <span className="min-w-0 wrap-anywhere">{choice.label}</span>
                  {choice.disabledReason && (
                    <span className="text-xs text-muted-foreground">{choice.disabledReason}</span>
                  )}
                </Button>
              </DrawerClose>
            ))}
          </div>
        </div>
      </DrawerContent>
    </Root>
  );
}
