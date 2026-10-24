"use client";
import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useThemeImport } from "@/hooks/domains/plugins/use-theme-import";
import { useTheme } from "@/components/theme/app-theme";
import { APPEARANCE_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/preferences";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import type { ThemeReview } from "@/lib/api/domains/themes-api";

// i18n-exempt: file extensions consumed by the browser upload control.
// i18n-exempt: illustrative JavaScript source for the theme syntax preview.
const SAMPLE = { declaration: "const ", assignment: "message = ", value: '"Hello"' };
const THEME_ACCEPT = ".json,.jsonc,.tar.gz,.tgz";
export function ThemeImportDialog({
  open,
  onOpenChange,
  entry,
  onInstalled,
}: {
  open: boolean;
  onOpenChange: (value: boolean) => void;
  entry?: MarketplaceEntry | null;
  onInstalled?: () => void;
}) {
  const { t } = useTranslation();
  const { refreshThemes } = useTheme();
  const install = useThemeImport();
  const [file, setFile] = useState<File | null>(null);
  const [base, setBase] = useState("auto");
  const { reset } = install;
  useEffect(() => {
    reset();
    setFile(null);
    setBase("auto");
  }, [open, entry?.id, entry?.version, reset]);
  const changeOpen = (value: boolean) => {
    if (install.busy) return;
    onOpenChange(value);
  };
  const body = (
    <ThemeImportBody
      install={install}
      entry={entry}
      base={base}
      setBase={setBase}
      setFile={setFile}
    />
  );
  const footer = (
    <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <Button
        variant="outline"
        className="min-h-11 cursor-pointer"
        disabled={install.busy}
        onClick={() => changeOpen(false)}
      >
        {t("common:close")}
      </Button>
      {install.installed ? (
        <Button asChild className="min-h-11">
          <Link href={APPEARANCE_SETTINGS_HREF} onClick={() => onOpenChange(false)}>
            {t("settings:appearance")}
          </Link>
        </Button>
      ) : (
        <>
          <Button
            variant="outline"
            data-testid="theme-review"
            className="min-h-11 cursor-pointer"
            disabled={install.busy || (!file && !entry)}
            onClick={() => void install.prepare(file, base === "auto" ? undefined : base, entry)}
          >
            {t("plugins:themeReview")}
          </Button>
          {install.review && (
            <Button
              data-testid="theme-confirm"
              className="min-h-11 cursor-pointer"
              disabled={install.busy}
              onClick={async () => {
                if (await install.commit()) {
                  refreshThemes();
                  onInstalled?.();
                }
              }}
            >
              {t("plugins:themeInstall")}
            </Button>
          )}
        </>
      )}
    </div>
  );
  return <ThemeImportFrame open={open} changeOpen={changeOpen} body={body} footer={footer} />;
}
function ThemeImportFrame({
  open,
  changeOpen,
  body,
  footer,
}: {
  open: boolean;
  changeOpen: (open: boolean) => void;
  body: ReactNode;
  footer: ReactNode;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={changeOpen}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col rounded-none">
          <DrawerHeader className="shrink-0">
            <DrawerTitle>{t("plugins:themeImport")}</DrawerTitle>
            <DrawerDescription>{t("plugins:themeImportDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          {footer}
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="flex max-h-[85dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="shrink-0 p-4 pr-10">
          <DialogTitle>{t("plugins:themeImport")}</DialogTitle>
          <DialogDescription>{t("plugins:themeImportDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        {footer}
      </DialogContent>
    </Dialog>
  );
}

function ThemeReviewContent({ review }: { review: ThemeReview }) {
  const { t } = useTranslation();
  return (
    <section data-testid="theme-review-content" className="space-y-3 text-sm">
      <h3 className="break-words font-semibold">
        {review.manifest.display_name} · {review.manifest.version}
      </h3>
      <p className="break-words">
        {review.manifest.author} {review.manifest.distribution.license}
      </p>
      <p>{t("plugins:themeDataOnly")}</p>
      {review.manifest.themes.map((variant) => {
        const d = review.definitions[variant.key];
        const editor = d.editor ?? {};
        const ui = d.ui ?? {};
        const syntax = d.syntax ?? {};
        return (
          <div
            key={variant.key}
            className="rounded-lg border p-4"
            style={{
              backgroundColor:
                editor.background ??
                ui.background ??
                (variant.base === "dark" ? "#1e1e1e" : "#ffffff"),
              color:
                editor.foreground ??
                ui.foreground ??
                (variant.base === "dark" ? "#d4d4d4" : "#333333"),
            }}
          >
            <p>
              {variant.label} ({variant.base})
            </p>
            <pre className="overflow-x-auto text-xs">
              <code>
                <span style={{ color: syntax.keyword?.foreground }}>{SAMPLE.declaration}</span>
                {SAMPLE.assignment}
                <span style={{ color: syntax.string?.foreground }}>{SAMPLE.value}</span>
                {";"}
              </code>
            </pre>
          </div>
        );
      })}
      <p>
        {t("plugins:themeReport", {
          mapped: review.report.mapped,
          approximated: review.report.approximated,
          ignored: review.report.ignored,
          fallback: review.report.fallback,
        })}
      </p>
      <p className="text-muted-foreground">{t("plugins:themeCompatibility")}</p>
      <ul className="space-y-1 break-words text-xs">
        {review.report.findings?.map((finding, index) => (
          <li key={index}>
            {finding.field}: {finding.message}
          </li>
        ))}
      </ul>
    </section>
  );
}

function ThemeImportBody({
  install,
  entry,
  base,
  setBase,
  setFile,
}: {
  install: ReturnType<typeof useThemeImport>;
  entry?: MarketplaceEntry | null;
  base: string;
  setBase: (value: string) => void;
  setFile: (file: File | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4">
      {entry ? (
        <p className="break-words font-medium">
          {entry.name} · {entry.version}
        </p>
      ) : (
        <>
          <label className="block space-y-2 text-sm" htmlFor="theme-import-file">
            <span>{t("plugins:themeFile")}</span>
            <Input
              id="theme-import-file"
              data-testid="theme-import-file"
              type="file"
              accept={THEME_ACCEPT}
              disabled={install.busy}
              onChange={(e) => {
                install.reset();
                setFile(e.target.files?.[0] ?? null);
              }}
            />
          </label>
          <Select
            value={base}
            onValueChange={(value) => {
              install.reset();
              setBase(value);
            }}
            disabled={install.busy}
          >
            <SelectTrigger className="min-h-11" aria-label={t("plugins:themeBase")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="auto">{t("plugins:themeBase")}</SelectItem>
              <SelectItem value="dark">{t("settings:dark")}</SelectItem>
              <SelectItem value="light">{t("settings:light")}</SelectItem>
            </SelectContent>
          </Select>
        </>
      )}
      {install.error && (
        <p role="alert" className="break-words text-sm text-destructive">
          {install.error}
        </p>
      )}
      {install.review && <ThemeReviewContent review={install.review} />}
      {install.installed && (
        <p data-testid="theme-installed" className="text-sm">
          {t("plugins:themeInstalled")}
        </p>
      )}
    </div>
  );
}
