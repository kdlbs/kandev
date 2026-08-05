import { emitSettingsTargetRequest, settingsTargetFromHash } from "./target";

export type SettingsDiscoveryPush = (href: string) => void;

export function navigateToSettingsDiscovery(href: string, push: SettingsDiscoveryPush): void {
  if (typeof window === "undefined") {
    push(href);
    return;
  }

  const destination = new URL(href, window.location.href);
  const samePage =
    destination.origin === window.location.origin &&
    destination.pathname === window.location.pathname &&
    destination.search === window.location.search;
  if (!samePage) {
    push(href);
    return;
  }

  const relativeHref = `${destination.pathname}${destination.search}${destination.hash}`;
  window.history.pushState(window.history.state, "", relativeHref);
  const targetId = settingsTargetFromHash(destination.hash);
  if (targetId) emitSettingsTargetRequest(targetId);
}
