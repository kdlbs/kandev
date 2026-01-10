'use client';

import { Sheet, SheetContent } from '@/components/ui/sheet';
import { SettingsSidebar } from './settings-sidebar';

type SettingsMobileNavProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function SettingsMobileNav({ open, onOpenChange }: SettingsMobileNavProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="left" className="w-64 p-0">
        <div className="pt-6">
          <SettingsSidebar
            className="border-0"
            onNavigate={() => onOpenChange(false)}
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}
