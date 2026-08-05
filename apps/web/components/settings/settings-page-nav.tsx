"use client";

import { SettingsTree } from "@/components/app-sidebar/sections/settings/settings-tree";

/**
 * The settings tree as touch rows: the nav sheet's page section on every
 * settings route, and the whole page at `/settings` on a phone. The wrapper
 * compacts the sidebar-styled tree; link clicks are handled by whatever hosts
 * it (`AppNavSheet` closes itself on any link click).
 */
export function SettingsPageNav({
  pathname,
  defaultOpenGroup,
}: {
  pathname: string;
  /** Group to open when `pathname` belongs to none — see `SettingsTree`. */
  defaultOpenGroup?: string;
}) {
  return (
    <div className="flex flex-col gap-0.5 [&_a]:min-h-10 [&_a]:text-sm [&_button]:min-h-10 [&_button]:text-sm [&_svg]:h-4 [&_svg]:w-4">
      <SettingsTree pathname={pathname} {...(defaultOpenGroup ? { defaultOpenGroup } : {})} />
    </div>
  );
}
