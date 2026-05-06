"use client";

import { type ReactNode } from "react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";

type MobilePickerSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  /** Optional trailing element rendered to the right of the title (e.g. a "+" CTA). */
  headerAction?: ReactNode;
  children: ReactNode;
};

/**
 * Bottom-sheet shell for picker patterns (sessions, terminals, repos). Wraps
 * shadcn Drawer with a consistent header layout — picker components only need
 * to render their list inside `children`.
 */
export function MobilePickerSheet({
  open,
  onOpenChange,
  title,
  description,
  headerAction,
  children,
}: MobilePickerSheetProps) {
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent>
        <DrawerHeader className="text-left pb-2">
          <div className="flex items-center justify-between gap-2">
            <DrawerTitle className="text-sm">{title}</DrawerTitle>
            {headerAction}
          </div>
          {description && <DrawerDescription>{description}</DrawerDescription>}
        </DrawerHeader>
        <div className="flex-1 min-h-0 overflow-y-auto px-2 pb-4">{children}</div>
      </DrawerContent>
    </Drawer>
  );
}
