"use client";

import { SettingsTree } from "@/components/app-sidebar/sections/settings/settings-tree";

/**
 * The settings tree as touch rows: the whole page at `/settings` on a phone,
 * where there is no sidebar to hold it. The wrapper compacts the sidebar-styled
 * tree, and the search field floats in thumb reach instead of sitting above a
 * list the user has to scroll past to reach it again.
 */
export function SettingsPageNav({ pathname }: { pathname: string }) {
  return (
    <div className="flex flex-col gap-0.5 [&_a]:min-h-11 [&_a]:text-sm [&_button]:min-h-11 [&_button]:text-sm [&_svg]:h-4 [&_svg]:w-4">
      <SettingsTree pathname={pathname} searchLayout="floating" />
    </div>
  );
}
