"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
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
  visitedTabs: ReadonlySet<string>;
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
  const [visitedTabs, setVisitedTabs] = useState<ReadonlySet<string>>(() => new Set([value]));

  useEffect(() => {
    setVisitedTabs((current) => {
      if (current.has(value)) return current;
      return new Set([...current, value]);
    });
  }, [value]);

  useEffect(() => {
    const targetId = settingsTargetFromHash(window.location.hash);
    if (targetId) emitSettingsTargetRequest(targetId);
  }, [value]);

  return (
    <SettingsTabsContext.Provider value={{ tabs, value, visitedTabs }}>
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
        "h-auto group-data-horizontal/tabs:h-auto justify-start gap-1 border border-border/70 bg-muted/40 p-[3px]",
        className,
      )}
    >
      {context.tabs.map((tab) => (
        <TabsTrigger
          key={tab.id}
          value={tab.id}
          className={cn(
            "h-7 min-w-24 flex-none cursor-pointer px-3 max-md:h-11 [@media(pointer:coarse)]:h-11",
            "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
            "data-[state=active]:border-border data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-sm",
            "dark:data-[state=active]:border-foreground/15 dark:data-[state=active]:bg-muted dark:data-[state=active]:text-foreground",
            "focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset focus-visible:outline-none",
            "transition-[color,background-color,border-color,box-shadow] duration-150 motion-reduce:transition-none",
          )}
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
  const visited = context.visitedTabs.has(value);
  return (
    <TabsContent
      value={value}
      forceMount
      aria-hidden={!active}
      className={cn("min-w-0 data-[state=inactive]:hidden", className)}
      data-testid={testId}
    >
      {visited ? children : null}
    </TabsContent>
  );
}
