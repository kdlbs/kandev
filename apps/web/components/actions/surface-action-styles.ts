import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type {
  PluginActionPresentation,
  PluginActionSurfaceName,
} from "@/components/plugins/plugin-action-surface";

export function surfaceActionClassName(
  surface: PluginActionSurfaceName,
  presentation: PluginActionPresentation | undefined,
  iconOnly: boolean,
): string {
  if (surface === "status-bar") {
    return "h-6 max-w-72 gap-1 px-1 text-[11px]";
  }
  if (surface === "status-drawer") {
    return "min-h-11 w-full justify-start gap-2 px-3 text-left";
  }
  if (surface === "sidebar") {
    if (presentation === "mobile") {
      return iconOnly ? "size-11 p-0" : "min-h-11 max-w-40 gap-1.5 px-2 text-left";
    }
    return iconOnly
      ? "size-6 [@media(pointer:coarse)]:size-11 rounded-md p-0 [&_svg]:size-3.5"
      : "h-6 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11 max-w-32 gap-1 px-1.5 text-[11px] [&_svg]:size-3.5";
  }
  if (presentation === "mobile") {
    return iconOnly ? "size-11 p-0" : "min-h-11 min-w-11 max-w-48 gap-1.5 px-2";
  }
  return iconOnly
    ? `${controlSizingClassName("icon")} p-0`
    : `${controlSizingClassName("standard")} max-w-48 gap-1.5 px-2`;
}

export function surfaceActionGlyphClassName(surface: PluginActionSurfaceName): string {
  switch (surface) {
    case "sidebar":
      return "size-3.5";
    case "status-bar":
      return "size-3";
    default:
      return "size-4";
  }
}

export function surfaceActionGroupClassName(
  surface: PluginActionSurfaceName,
  presentation: PluginActionPresentation | undefined,
): string {
  if (surface === "status-bar") {
    return "inline-flex min-w-0 max-w-72 flex-nowrap items-center gap-0.5 [&>[data-slot=surface-action]]:min-w-0 [&>[data-slot=surface-action]]:shrink";
  }
  if (surface === "status-drawer") return "flex w-full flex-col gap-1";
  if (presentation === "mobile") {
    return "inline-flex min-w-0 max-w-full flex-wrap items-center gap-2";
  }
  if (surface === "topbar") {
    return "inline-flex min-w-0 max-w-full items-center gap-1 [@media(pointer:coarse)]:flex-wrap";
  }
  return "inline-flex min-w-0 items-center gap-0.5";
}
