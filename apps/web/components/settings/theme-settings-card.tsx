"use client";
import { useState } from "react";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsRow } from "./settings-group";
import { settingsControlClassName } from "./settings-control";
import { useTheme } from "@/components/theme/app-theme";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { themeReference } from "@/lib/theme/theme-resolver";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import type { Theme } from "@/lib/settings/types";
import { ThemeImportDialog } from "./plugins/theme-import-dialog";

export function ThemeSettingsCard({
  theme,
  isDirty,
  onChange,
}: {
  theme: Theme;
  isDirty: boolean;
  onChange: (theme: Theme) => void;
}) {
  const { t } = useTranslation();
  const { themeInventory, effectiveTheme, themeError, refreshThemes } = useTheme();
  const canManage = useIsAdmin();

  const [open, setOpen] = useState(false);
  const variants = themeInventory?.themes ?? [];
  const missing =
    theme.startsWith("theme:") &&
    !variants.some((d) => themeReference(d.package_id, d.key) === theme);
  return (
    <div className="space-y-2">
      <SettingsRow
        isDirty={isDirty}
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.colorTheme}
        data-testid="theme-settings-card"
        label={t("settings:colorTheme")}
        controlId="theme"
        control={
          <Select value={theme} onValueChange={(value) => onChange(value as Theme)}>
            <SelectTrigger
              id="theme"
              className={settingsControlClassName()}
              data-settings-dirty={isDirty}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="system">{t("common:system")}</SelectItem>
              <SelectItem value="light">{t("settings:light")}</SelectItem>
              <SelectItem value="dark">{t("settings:dark")}</SelectItem>
              {missing && (
                <SelectItem value={theme} disabled>
                  {t("plugins:themeUnavailable")}
                </SelectItem>
              )}
              {variants.map((d) => (
                <SelectItem
                  key={themeReference(d.package_id, d.key)}
                  value={themeReference(d.package_id, d.key)}
                >
                  {d.package_name} / {d.label} ({d.package_id})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
      {(themeError || effectiveTheme?.status !== "ready") && (
        <div role="status" className="flex flex-wrap items-center gap-2 text-sm">
          <span>
            {themeError ??
              (effectiveTheme?.status === "unavailable"
                ? t("plugins:themeUnavailable")
                : t("common:loading"))}
          </span>
          <Button variant="outline" className="min-h-11 cursor-pointer" onClick={refreshThemes}>
            {t("plugins:retry")}
          </Button>
        </div>
      )}
      {
        <div className="flex flex-wrap gap-2 pb-3">
          {canManage && (
            <Button
              variant="outline"
              data-testid="appearance-theme-import"
              className="min-h-11 cursor-pointer"
              onClick={() => setOpen(true)}
            >
              {t("plugins:themeImport")}
            </Button>
          )}
          <Button variant="outline" asChild className="min-h-11">
            <Link href="/settings/plugins?tab=themes">{t("plugins:themeBrowse")}</Link>
          </Button>
        </div>
      }
      {canManage && <ThemeImportDialog open={open} onOpenChange={setOpen} />}
    </div>
  );
}
