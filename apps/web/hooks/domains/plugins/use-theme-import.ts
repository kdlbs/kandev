import { useCallback, useEffect, useRef, useState } from "react";
import {
  prepareThemeFile,
  prepareThemeURL,
  commitThemeReview,
  type ThemeReview,
} from "@/lib/api/domains/themes-api";
import type { MarketplaceEntry } from "@/lib/types/plugins";
import { t } from "@/lib/i18n";
import { useOptionalAppStoreApi } from "@/components/state-provider";

export function useThemeImport() {
  const store = useOptionalAppStoreApi();
  const epoch = useRef(0);
  const [review, setReview] = useState<ThemeReview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [installed, setInstalled] = useState(false);
  const reset = useCallback(() => {
    epoch.current++;
    setReview(null);
    setError(null);
    setBusy(false);
    setInstalled(false);
  }, []);
  useEffect(
    () => () => {
      epoch.current++;
    },
    [],
  );
  const prepare = async (file: File | null, base?: string, entry?: MarketplaceEntry | null) => {
    const request = ++epoch.current;
    setBusy(true);
    setError(null);
    setReview(null);
    try {
      let result: ThemeReview | null = null;
      if (entry)
        result = await prepareThemeURL(
          entry.package_url,
          entry.id,
          entry.version,
          entry.package_sha256,
        );
      else if (file) result = await prepareThemeFile(file, base);
      if (request === epoch.current) setReview(result);
    } catch (err) {
      if (request === epoch.current)
        setError(err instanceof Error ? err.message : t("plugins:failedToLoadPlugins"));
    } finally {
      if (request === epoch.current) setBusy(false);
    }
  };
  const commit = async () => {
    if (!review || busy) return false;
    const request = epoch.current;
    setBusy(true);
    setError(null);
    try {
      const result = await commitThemeReview(review.id);
      if (request === epoch.current) {
        setInstalled(true);
        store?.getState().upsertPlugin(result.plugin);
      }
      return true;
    } catch (err) {
      if (request === epoch.current)
        setError(err instanceof Error ? err.message : t("plugins:failedToLoadPlugins"));
      return false;
    } finally {
      if (request === epoch.current) setBusy(false);
    }
  };
  return { review, busy, error, installed, reset, prepare, commit };
}
