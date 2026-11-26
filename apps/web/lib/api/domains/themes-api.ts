import { ApiError, fetchJson } from "../client";
import { getBackendConfig } from "@/lib/config";
import { t } from "@/lib/i18n";
import { parseThemeDefinition } from "@/lib/theme/theme-schema";
import type { ThemeDescriptor, ThemeInventory, ThemeDefinition } from "@/lib/theme/theme-types";
import type { InstallResult } from "./plugins-api";

export type ThemeReview = {
  id: string;
  digest: string;
  expires_at: string;
  manifest: {
    id: string;
    display_name: string;
    version: string;
    author?: string;
    distribution: { license?: string };
    themes: { key: string; label: string; base: "light" | "dark" }[];
  };
  definitions: Record<string, ThemeDefinition>;
  report: {
    mapped: number;
    approximated: number;
    ignored: number;
    fallback: number;
    findings: { kind: string; field: string; message: string }[];
  };
};
export const listThemes = () =>
  fetchJson<ThemeInventory>("/api/plugins/themes", { cache: "no-store" });
export async function getThemeDefinition(descriptor: ThemeDescriptor): Promise<ThemeDefinition> {
  const raw = await fetchJson<unknown>(
    `/api/plugins/themes/${encodeURIComponent(descriptor.package_id)}/${encodeURIComponent(descriptor.key)}?digest=${encodeURIComponent(descriptor.digest)}`,
    { cache: "no-store" },
  );
  const definition = parseThemeDefinition(raw);
  if (!definition || definition.base !== descriptor.base)
    throw new Error(t("plugins:failedToLoadPlugins"));
  return definition;
}
export async function prepareThemeFile(file: File, base?: string): Promise<ThemeReview> {
  const form = new FormData();
  form.append("package", file);
  if (base) form.append("base", base);
  const response = await fetch(`${getBackendConfig().apiBaseUrl}/api/plugins/themes/prepare`, {
    method: "POST",
    body: form,
    credentials: "include",
  });
  const body = await response.json();
  if (!response.ok)
    throw new ApiError(
      typeof body.error === "string" ? body.error : t("plugins:failedToLoadPlugins"),
      response.status,
      body,
    );
  return body as ThemeReview;
}
export const prepareThemeURL = (url: string, id: string, version: string, digest: string) =>
  fetchJson<ThemeReview>("/api/plugins/themes/prepare", {
    init: { method: "POST", body: JSON.stringify({ url, id, version, digest }) },
  });
export const commitThemeReview = (id: string) =>
  fetchJson<InstallResult>("/api/plugins/themes/commit", {
    init: { method: "POST", body: JSON.stringify({ id }) },
  });
