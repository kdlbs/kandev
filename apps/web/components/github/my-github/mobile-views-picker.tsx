import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { PresetsSidebar, type PresetsSidebarProps } from "./presets-sidebar";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";

export function MobileViewsPicker({
  title,
  onSelect,
  onSaveCurrent,
  ...props
}: PresetsSidebarProps & { title: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const saveAfterClose = useRef(false);
  const saveFrame = useRef(0);
  useEffect(
    () => () => {
      saveAfterClose.current = false;
      cancelAnimationFrame(saveFrame.current);
    },
    [],
  );
  return (
    <MobileConfirmationHost open={open} surface="drawer">
      {({ contentProps }) => (
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerTrigger asChild>
            <Button
              variant="outline"
              className="h-11 min-w-0 w-full justify-between gap-2 text-sm font-normal"
              data-testid="github-mobile-menu-button"
              aria-label={t("github:viewPicker", { view: title })}
            >
              <span className="min-w-0 truncate">
                <span className="text-muted-foreground">{t("github:views")}:</span>{" "}
                <span className="font-medium" data-testid="github-list-toolbar-title">
                  {title}
                </span>
              </span>
              <IconChevronDown className="h-4 w-4 shrink-0" />
            </Button>
          </DrawerTrigger>
          <DrawerContent
            className="h-[88dvh] max-h-[88dvh] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]"
            data-testid="github-mobile-sidebar"
            aria-describedby={undefined}
            {...contentProps}
            onCloseAutoFocus={() => {
              if (!saveAfterClose.current) return;
              saveAfterClose.current = false;
              saveFrame.current = requestAnimationFrame(onSaveCurrent);
            }}
          >
            <MobileConfirmationHostBody>
              <DrawerHeader className="flex-row shrink-0 items-center justify-between px-3 py-2">
                <DrawerTitle>{t("github:views")}</DrawerTitle>
                <DrawerClose asChild>
                  <Button variant="ghost">{t("task:done")}</Button>
                </DrawerClose>
              </DrawerHeader>
              <PresetsSidebar
                {...props}
                onSelect={(selection) => {
                  onSelect(selection);
                  if (selection.source !== "kind-switch") setOpen(false);
                }}
                onSaveCurrent={() => {
                  saveAfterClose.current = true;
                  setOpen(false);
                }}
              />
            </MobileConfirmationHostBody>
          </DrawerContent>
        </Drawer>
      )}
    </MobileConfirmationHost>
  );
}
