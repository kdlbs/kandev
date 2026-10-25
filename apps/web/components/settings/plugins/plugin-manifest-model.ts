import type { PluginWebhook } from "@/lib/types/plugins";

export function isPublicWebhook(apiVersion: number, access: PluginWebhook["access"]): boolean {
  if (access !== undefined) return access === "public";
  return apiVersion === 1;
}
