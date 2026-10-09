"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";

/**
 * Reveals entries whose name begins with a dot in the directory being browsed.
 *
 * A switch names the thing it controls and reports its own state, so the label
 * stays a short stable noun instead of a Show/Hide verb pair, and the control
 * carries no `aria-pressed` to contradict a name that changes with the state.
 *
 * It is pinned to the breadcrumb row's trailing edge so a long path scrolls
 * behind it, and it reads the same stored preference the listing hook reads, so
 * no directory browser can show a different state. The label wraps the switch so
 * the whole band is the tap target rather than the switch alone.
 */
export function ShowHiddenToggle({ touchRows = false }: { touchRows?: boolean }) {
  const { t } = useTranslation();
  const showHidden = useAppStore((state) => state.directoryBrowserShowHidden);
  const setShowHidden = useAppStore((state) => state.setDirectoryBrowserShowHidden);
  return (
    <label
      data-testid="directory-browser-show-hidden"
      className={cn(
        "flex shrink-0 cursor-pointer items-center gap-1.5 border-l border-border px-2",
        "text-[11px] whitespace-nowrap text-muted-foreground",
        "max-md:min-h-12",
        "[@media(pointer:coarse)]:min-h-11",
        touchRows && "min-h-11",
      )}
    >
      <Switch
        size="sm"
        checked={showHidden}
        onCheckedChange={setShowHidden}
        aria-label={t("common:hiddenFolders")}
      />
      <span>{t("common:hiddenFolders")}</span>
    </label>
  );
}
