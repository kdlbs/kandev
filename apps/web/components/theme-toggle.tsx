"use client";

import { useTheme } from "@/components/theme/app-theme";
import { IconMoon, IconSun } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";

export function ThemeToggle() {
  const { t } = useTranslation();
  const { theme, setTheme } = useTheme();
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
      onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
      className="h-9 w-9 p-0"
    >
      {theme === "dark" ? <IconSun className="h-4 w-4" /> : <IconMoon className="h-4 w-4" />}
      <span className="sr-only">{t("sidebar:toggleTheme")}</span>
    </Button>
  );
}
