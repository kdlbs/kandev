"use client";

import { createContext, useContext, useEffect, type ReactNode } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { cn } from "@/lib/utils";
import { emitSettingsTargetRequest, settingsTargetFromHash } from "@/lib/settings-discovery/target";

export type SettingsTabOption = {
  id: string;
  label: ReactNode;
};

type SettingsTabsContextValue = {
  tabs: readonly SettingsTabOption[];
  value: string;
};

const SettingsTabsContext = createContext<SettingsTabsContextValue | null>(null);

export function SettingsTabs({
  tabs,
  value,
  onValueChange,
  children,
}: {
  tabs: readonly SettingsTabOption[];
  value: string;
  onValueChange: (value: string) => void;
  children: ReactNode;
}) {
  useEffect(() => {
    const targetId = settingsTargetFromHash(window.location.hash);
    if (targetId) emitSettingsTargetRequest(targetId);
  }, [value]);

  return (
    <SettingsTabsContext.Provider value={{ tabs, value }}>
      <Tabs value={value} onValueChange={onValueChange} activationMode="manual" className="min-w-0">
        {children}
      </Tabs>
    </SettingsTabsContext.Provider>
  );
}

export function SettingsTabsList({
  ariaLabel,
  className,
}: {
  ariaLabel: string;
  className?: string;
}) {
  const context = useContext(SettingsTabsContext);
  // i18n-exempt: programmer error for an invalid component composition.
  if (!context) throw new Error("SettingsTabsList must be used inside SettingsTabs");
  return (
    <TabsList
      aria-label={ariaLabel}
      className={cn(
        "min-w-0 max-w-full overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        "h-8 max-md:h-11 [@media(pointer:coarse)]:h-11",
        className,
      )}
    >
      {context.tabs.map((tab) => (
        <TabsTrigger
          key={tab.id}
          value={tab.id}
          className="h-7 min-w-24 flex-none cursor-pointer px-3 max-md:h-11 [@media(pointer:coarse)]:h-11"
        >
          {tab.label}
        </TabsTrigger>
      ))}
    </TabsList>
  );
}

export function SettingsTabsPanel({
  value,
  children,
  className,
  testId,
}: {
  value: string;
  children: ReactNode;
  className?: string;
  testId?: string;
}) {
  const context = useContext(SettingsTabsContext);
  // i18n-exempt: programmer error for an invalid component composition.
  if (!context) throw new Error("SettingsTabsPanel must be used inside SettingsTabs");
  const active = context.value === value;
  return (
    <TabsContent
      value={value}
      forceMount
      aria-hidden={!active}
      className={cn("min-w-0 data-[state=inactive]:hidden", className)}
      data-testid={testId}
    >
      {children}
    </TabsContent>
  );
}
