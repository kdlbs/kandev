export const SSH_CONNECTIONS_SETTINGS_HREF = "/settings/executors/ssh-connections";

const queryKeys = [
  "tab",
  "id",
  "new",
  "return_connection",
  "return_identity_connection",
  "return_section",
  "select_identity",
  "section",
  "mode",
] as const;
type ManagerQueryKey = (typeof queryKeys)[number];

/** Carry public setup context only; credentials never belong in navigation. */
export function sshConnectionSettingsPath(
  current: URLSearchParams = new URLSearchParams(),
  changes: Partial<Record<ManagerQueryKey, string | null>> = {},
): string {
  const query = new URLSearchParams();
  for (const key of queryKeys) {
    const value = key in changes ? changes[key] : current.get(key);
    if (key === "return_section" && !["access", "diagnostics"].includes(value ?? "")) continue;
    if (key === "section" && !["overview", "access", "diagnostics"].includes(value ?? "")) continue;
    if (key === "mode" && value !== "edit") continue;
    if (value) query.set(key, value);
  }
  return `${SSH_CONNECTIONS_SETTINGS_HREF}${query.size ? `?${query}` : ""}`;
}
