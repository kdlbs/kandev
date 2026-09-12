import type { ReactNode } from "react";
import { Drawer, DrawerContent } from "@kandev/ui/drawer";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";

export function TaskSwitcherDrawer({
  open,
  onOpenChange,
  onCloseAutoFocus,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus?: (event: Event) => void;
  children: ReactNode;
}) {
  return (
    <MobileConfirmationHost open={open} surface="drawer">
      {({ contentProps }) => (
        <Drawer open={open} onOpenChange={onOpenChange}>
          <DrawerContent
            onCloseAutoFocus={onCloseAutoFocus}
            {...contentProps}
            className="h-[88dvh] max-h-[88dvh] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]"
          >
            <MobileConfirmationHostBody>{children}</MobileConfirmationHostBody>
          </DrawerContent>
        </Drawer>
      )}
    </MobileConfirmationHost>
  );
}
