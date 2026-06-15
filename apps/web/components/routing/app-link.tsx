"use client";

import type { AnchorHTMLAttributes, MouseEvent, ReactNode } from "react";

type AppLinkHref = string | URL;

export type AppLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> & {
  href: AppLinkHref;
  children?: ReactNode;
};

const LOCATION_CHANGE_EVENT = "kandev:navigation";

export default function Link({ href, onClick, ...props }: AppLinkProps) {
  const resolvedHref = href.toString();

  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event);
    if (shouldUseBrowserNavigation(event, resolvedHref)) return;

    event.preventDefault();
    window.history.pushState({}, "", resolvedHref);
    window.dispatchEvent(new Event(LOCATION_CHANGE_EVENT));
  };

  return <a {...props} href={resolvedHref} onClick={handleClick} />;
}

function shouldUseBrowserNavigation(event: MouseEvent<HTMLAnchorElement>, href: string): boolean {
  if (event.defaultPrevented) return true;
  if (event.button !== 0) return true;
  if (event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return true;

  const target = event.currentTarget.getAttribute("target");
  if (target && target !== "_self") return true;

  return isExternalHref(href);
}

function isExternalHref(href: string): boolean {
  if (href.startsWith("#")) return false;
  try {
    const url = new URL(href, window.location.href);
    return url.origin !== window.location.origin;
  } catch {
    return false;
  }
}
