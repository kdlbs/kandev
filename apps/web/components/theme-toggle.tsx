"use client";

import { useTheme } from "@/components/theme/app-theme";
import { getThemeToggleLabelKey, getThemeToggleTarget } from "@/components/theme/theme-toggle";
import { IconMoon, IconSun } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";

export function ThemeToggle() {
  const { t } = useTranslation();
  const { resolvedTheme, setTheme } = useTheme();
  const targetTheme = getThemeToggleTarget(resolvedTheme);
  const labelKey = getThemeToggleLabelKey(resolvedTheme);
  const mounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );

  if (!mounted) {
    return null;
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => setTheme(targetTheme)}
      className="h-9 w-9 p-0"
      aria-label={t(labelKey)}
      aria-pressed={resolvedTheme === "dark"}
    >
      {resolvedTheme === "dark" ? (
        <IconSun className="h-4 w-4" />
      ) : (
        <IconMoon className="h-4 w-4" />
      )}
    </Button>
  );
}
