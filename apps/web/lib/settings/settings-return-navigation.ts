export function isSettingsRoute(pathname: string | null): boolean {
  return pathname === "/settings" || Boolean(pathname?.startsWith("/settings/"));
}

export function settingsReturnHref(
  current: string | null,
  fromHref: string,
  toHref: string,
): string | null {
  const pathname = (href: string) => href.split(/[?#]/, 1)[0];
  if (!isSettingsRoute(pathname(toHref))) return null;
  if (isSettingsRoute(pathname(fromHref))) return current;
  return fromHref.startsWith("/") && !fromHref.startsWith("//") ? fromHref : null;
}
