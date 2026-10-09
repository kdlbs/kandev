import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
  DrawerClose,
} from "@kandev/ui/drawer";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@kandev/ui/sheet";

export function AppNavSurface({
  isMobile,
  open,
  onOpenChange,
  onCloseAutoFocus,
  trigger,
  children,
}: {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event: Event) => void;
  trigger: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          data-testid="app-nav-sheet"
          data-legacy-testid="mobile-home-menu-card"
          aria-describedby={undefined}
          onCloseAutoFocus={onCloseAutoFocus}
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            (event.currentTarget as HTMLElement | null)?.focus();
          }}
          tabIndex={-1}
          className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] outline-none"
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background shadow-2xl shadow-black/20">
            <DrawerHeader className="shrink-0 !flex-row items-center justify-between border-b border-border/70 px-4 py-1 text-left">
              <DrawerTitle>{t("common:menu")}</DrawerTitle>
              <DrawerClose asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 cursor-pointer"
                  aria-label={t("common:close")}
                >
                  <IconX className="size-4" />
                </Button>
              </DrawerClose>
            </DrawerHeader>
            {children}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetTrigger asChild>{trigger}</SheetTrigger>
      <SheetContent
        onCloseAutoFocus={onCloseAutoFocus}
        side="left"
        aria-describedby={undefined}
        className="flex w-80 max-w-[85vw] flex-col overflow-hidden p-0"
        data-testid="app-nav-sheet"
        data-legacy-testid="settings-mobile-menu"
      >
        <SheetHeader className="border-b px-4 py-3 text-left">
          <SheetTitle>{t("common:menu")}</SheetTitle>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}
