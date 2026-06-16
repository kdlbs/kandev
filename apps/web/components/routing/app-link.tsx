"use client";

import { forwardRef } from "react";
import type { AnchorHTMLAttributes, MouseEvent, ReactNode } from "react";

import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";

type AppLinkHref = string | URL;

export type AppLinkProps = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href"> & {
  href: AppLinkHref;
  children?: ReactNode;
};

const Link = forwardRef<HTMLAnchorElement, AppLinkProps>(function Link(
  { href, onClick, ...props },
  ref,
) {
  const resolvedHref = href.toString();

  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event);
    if (shouldUseBrowserNavigation(event, resolvedHref)) return;

    event.preventDefault();
    window.history.pushState({}, "", resolvedHref);
    window.scrollTo({ top: 0, left: 0 });
    window.dispatchEvent(new Event(LOCATION_CHANGE_EVENT));
  };

  return <a {...props} ref={ref} href={resolvedHref} onClick={handleClick} />;
});

export default Link;

function shouldUseBrowserNavigation(event: MouseEvent<HTMLAnchorElement>, href: string): boolean {
  if (event.defaultPrevented) return true;
  if (event.button !== 0) return true;
  if (event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return true;

  const target = event.currentTarget.getAttribute("target");
  if (target && target !== "_self") return true;
  if (href.startsWith("#")) return true;

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
