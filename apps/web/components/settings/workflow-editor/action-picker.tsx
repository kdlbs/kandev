"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { settingsControlClassName } from "@/components/settings/settings-control";
import type { WorkflowActionDescriptor } from "@/lib/workflows/workflow-action-catalog";

const ADD_ACTION_LABEL_KEY = "workflows:addAction";

export function DesktopActionPicker({
  catalog,
  onAdd,
}: {
  catalog: readonly WorkflowActionDescriptor[];
  onAdd: (type: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <IconPlus className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
      <select
        aria-label={t(ADD_ACTION_LABEL_KEY)}
        className={settingsControlClassName(
          "min-w-0 max-w-sm flex-1 rounded-md border border-input bg-background px-2 text-sm",
        )}
        value=""
        onChange={(event) => {
          if (event.target.value) onAdd(event.target.value);
        }}
      >
        <option value="" disabled>
          {t(ADD_ACTION_LABEL_KEY)}
        </option>
        {catalog.map((item) => (
          <option key={item.type} value={item.type}>
            {t(item.labelKey)}
          </option>
        ))}
      </select>
    </div>
  );
}

export function MobileActionPicker({
  catalog,
  onAdd,
}: {
  catalog: readonly WorkflowActionDescriptor[];
  onAdd: (type: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <Button
        type="button"
        variant="outline"
        className="min-h-11 w-full cursor-pointer"
        onClick={() => setOpen(true)}
      >
        <IconPlus className="mr-1.5 h-4 w-4" />
        {t(ADD_ACTION_LABEL_KEY)}
      </Button>
      <DrawerContent data-testid="workflow-mobile-action-picker">
        <DrawerHeader>
          <DrawerTitle>{t(ADD_ACTION_LABEL_KEY)}</DrawerTitle>
          <DrawerDescription>{t("workflows:chooseActionDescription")}</DrawerDescription>
        </DrawerHeader>
        <div className="grid gap-2 overflow-y-auto px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {catalog.map((item) => (
            <Button
              key={item.type}
              type="button"
              variant="outline"
              className="min-h-11 cursor-pointer justify-start"
              onClick={() => {
                onAdd(item.type);
                setOpen(false);
              }}
            >
              {t(item.labelKey)}
            </Button>
          ))}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
