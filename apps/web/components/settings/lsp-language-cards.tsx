"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { Switch } from "@kandev/ui/switch";
import { readBootPayload } from "@/src/boot-payload";
import { Trans, useTranslation } from "react-i18next";
import {
  LSP_LANGUAGE_OPTIONS,
  lspAutoInstallConfigurable,
  lspLanguageDisplayLabel,
} from "./lsp-language-options";

// i18n-exempt: shell command the user runs verbatim.
const NPM_INSTALL_COMMAND = "npm install";

type LspLanguageCardsProps = {
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  baselineLspAutoStart: string[];
  baselineLspAutoInstall: string[];
  lspStatusHiddenLanguages: string[];
  baselineLspStatusHiddenLanguages: string[];
  toggleAutoStart: (langId: string, checked: boolean) => void;
  toggleAutoInstall: (langId: string, checked: boolean) => void;
  toggleStatusVisibility: (langId: string, visible: boolean) => void;
};

function LspLanguageCard({
  language,
  preferenceLanguages,
  lspAutoStartLanguages,
  lspAutoInstallLanguages,
  baselineLspAutoStart,
  baselineLspAutoInstall,
  lspStatusHiddenLanguages,
  baselineLspStatusHiddenLanguages,
  toggleAutoStart,
  toggleAutoInstall,
  toggleStatusVisibility,
}: LspLanguageCardsProps & {
  language: (typeof LSP_LANGUAGE_OPTIONS)[number];
  preferenceLanguages: readonly string[];
}) {
  const { t } = useTranslation();
  const languageLabel = lspLanguageDisplayLabel(language, (key, options) => t(key, options));
  const autoInstallConfigurable = lspAutoInstallConfigurable(language, preferenceLanguages);
  const autoStartDirty =
    lspAutoStartLanguages.includes(language.id) !== baselineLspAutoStart.includes(language.id);
  const autoInstallDirty =
    autoInstallConfigurable &&
    lspAutoInstallLanguages.includes(language.id) !== baselineLspAutoInstall.includes(language.id);
  const statusVisibilityDirty =
    lspStatusHiddenLanguages.includes(language.id) !==
    baselineLspStatusHiddenLanguages.includes(language.id);

  return (
    <div
      className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-2.5"
      data-settings-dirty={autoStartDirty || autoInstallDirty || statusVisibilityDirty}
      data-testid={`lsp-language-card-${language.id}`}
    >
      <div>
        <div className="text-sm font-medium text-foreground">{languageLabel}</div>
        <div className="text-xs text-muted-foreground">{language.binary}</div>
      </div>
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{t("settings:autoStart")}</span>
        <Switch
          checked={lspAutoStartLanguages.includes(language.id)}
          onCheckedChange={(checked) => toggleAutoStart(language.id, checked === true)}
          data-settings-dirty={autoStartDirty}
          data-testid={`lsp-auto-start-${language.id}`}
          aria-label={t("settings:autoStartLanguageServer", { language: languageLabel })}
        />
      </div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">{t("settings:showInTaskStatus")}</span>
        <Switch
          checked={!lspStatusHiddenLanguages.includes(language.id)}
          onCheckedChange={(checked) => toggleStatusVisibility(language.id, checked === true)}
          data-settings-dirty={statusVisibilityDirty}
          data-testid={`lsp-status-visible-${language.id}`}
          aria-label={t("settings:showLanguageInTaskStatus", { language: languageLabel })}
        />
      </div>
      <div className="flex items-center gap-2">
        {autoInstallConfigurable && (
          <Checkbox
            id={`lsp-install-${language.id}`}
            checked={lspAutoInstallLanguages.includes(language.id)}
            onCheckedChange={(checked) => toggleAutoInstall(language.id, checked === true)}
            className="h-3.5 w-3.5"
            data-settings-dirty={autoInstallDirty}
            data-testid={`lsp-auto-install-${language.id}`}
          />
        )}
        {autoInstallConfigurable ? (
          <label
            htmlFor={`lsp-install-${language.id}`}
            className="text-xs text-muted-foreground cursor-pointer"
          >
            {t("settings:autoInstallIfNotFound")}
          </label>
        ) : (
          <span className="text-xs text-muted-foreground">
            {t("settings:manualInstallRequired")}
          </span>
        )}
      </div>
      <p
        className="text-[11px] leading-relaxed text-muted-foreground"
        data-testid={`lsp-install-guidance-${language.id}`}
      >
        {t(language.installHintKey, language.installHintValues)}
      </p>
    </div>
  );
}

function autoInstallPreferenceLanguages(): readonly string[] {
  if (typeof window === "undefined") return [];
  return readBootPayload(window).runtime?.lspAutoInstallPreferenceLanguages ?? [];
}

export function LspLanguageCards(props: LspLanguageCardsProps) {
  const { t } = useTranslation();
  const preferenceLanguages = autoInstallPreferenceLanguages();
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">{t("settings:languageServers")}</div>
        <div className="text-xs text-muted-foreground">
          {t("settings:autoStartLanguageServersWhenOpening")}
          <br />
          <Trans
            i18nKey="settings:whenEnabledInstallYourProjectS"
            values={{ command: NPM_INSTALL_COMMAND }}
          >
            When enabled, install your project&apos;s dependencies (e.g.{" "}
            <code className="text-[11px] bg-muted px-1 rounded">{NPM_INSTALL_COMMAND}</code> via
            repository setup scripts) to avoid missing type errors.
          </Trans>
        </div>
        <p
          className="mt-1 text-[11px] leading-relaxed text-muted-foreground"
          data-testid="lsp-status-visibility-description"
        >
          {t("settings:lspStatusVisibilityDescription")}
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {LSP_LANGUAGE_OPTIONS.map((language) => (
          <LspLanguageCard
            key={language.id}
            language={language}
            preferenceLanguages={preferenceLanguages}
            {...props}
          />
        ))}
      </div>
    </div>
  );
}
