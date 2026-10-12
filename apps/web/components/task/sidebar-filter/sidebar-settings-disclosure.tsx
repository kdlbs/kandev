"use client";

import {
  DisplaySettingsDisclosure,
  type DisplaySettingsDisclosureProps,
} from "@/components/display-settings-disclosure";

type Props = Omit<DisplaySettingsDisclosureProps, "summaryClassName" | "touchTargets">;

/** Sidebar wrapper retaining the existing disclosure API and test ids. */
export function SidebarSettingsDisclosure(props: Props) {
  return <DisplaySettingsDisclosure {...props} summaryClassName="truncate" />;
}
