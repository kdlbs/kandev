export function isSettingsRoute(pathname: string | null): boolean {
  return pathname === "/settings" || Boolean(pathname?.startsWith("/settings/"));
}
