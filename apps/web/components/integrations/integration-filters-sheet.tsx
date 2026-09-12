"use client";

import type { ReactNode } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

/** Provider state stays with the caller; this shell owns only responsive presentation. */
export function IntegrationFiltersSheet({
  open,
  onOpenChange,
  title,
  testId,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  testId: string;
  children: ReactNode;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  return (
    <MobileConfirmationHost open={open} surface={isMobile ? "drawer" : "dialog"}>
      {({ active, contentProps }) =>
        isMobile ? (
          <Drawer open={open} onOpenChange={onOpenChange}>
            <DrawerContent
              className="overflow-hidden data-[vaul-drawer-direction=bottom]:max-h-[80dvh]"
              data-testid={testId}
              aria-describedby={undefined}
              {...contentProps}
            >
              <MobileConfirmationHostBody>
                <DrawerHeader className="shrink-0 text-left pb-2">
                  <DrawerTitle>{title}</DrawerTitle>
                </DrawerHeader>
                <div
                  data-testid="integration-filters-scroll"
                  className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-[calc(1rem+env(safe-area-inset-bottom))]"
                >
                  {children}
                </div>
              </MobileConfirmationHostBody>
            </DrawerContent>
          </Drawer>
        ) : (
          <Sheet open={open} onOpenChange={onOpenChange}>
            <SheetContent
              side="right"
              className="w-full sm:max-w-sm overflow-hidden p-0"
              data-testid={testId}
              showCloseButton={!active}
              aria-describedby={undefined}
              {...contentProps}
            >
              <MobileConfirmationHostBody>
                <div className="min-h-0 flex-1 overflow-y-auto">
                  <SheetHeader className="px-4 pt-4 pb-2">
                    <SheetTitle>{title}</SheetTitle>
                  </SheetHeader>
                  {children}
                </div>
              </MobileConfirmationHostBody>
            </SheetContent>
          </Sheet>
        )
      }
    </MobileConfirmationHost>
  );
}
