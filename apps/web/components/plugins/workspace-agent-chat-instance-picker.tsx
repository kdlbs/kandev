"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import type { PluginManagedConversationInstance } from "@kandev/plugin-sdk";

export function WorkspaceAgentChatInstancePicker({
  instances,
  selectedKey,
  onSelect,
}: {
  instances: readonly PluginManagedConversationInstance[];
  selectedKey: string;
  onSelect(instanceKey: string): void;
}) {
  const { t } = useTranslation("plugins");
  const [open, setOpen] = useState(false);
  const selected = instances.find((instance) => instance.key === selectedKey);
  if (instances.length === 0) return null;

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 max-w-[45vw] justify-between gap-2 truncate"
          aria-label={t("managedChatChooseInstance")}
          data-testid="managed-chat-instance-picker"
        >
          <span className="truncate">{selected?.label ?? t("managedChatChooseInstance")}</span>
          <span aria-hidden="true">⌄</span>
        </Button>
      </DrawerTrigger>
      <DrawerContent className="max-h-[80vh]">
        <DrawerHeader className="sticky top-0 z-10 border-b bg-background text-left">
          <DrawerTitle>{t("managedChatChooseInstance")}</DrawerTitle>
        </DrawerHeader>
        <div className="overflow-y-auto p-3">
          {instances.map((instance) => (
            <Button
              key={instance.key}
              type="button"
              variant={instance.key === selectedKey ? "secondary" : "ghost"}
              className="min-h-11 w-full justify-start"
              aria-pressed={instance.key === selectedKey}
              onClick={() => {
                onSelect(instance.key);
                setOpen(false);
              }}
            >
              {instance.label}
            </Button>
          ))}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
