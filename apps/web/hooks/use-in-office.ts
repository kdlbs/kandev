"use client";

import { usePathname } from "next/navigation";
import { useFeature } from "@/hooks/domains/features/use-feature";

/**
 * True only while the user is actually inside the Office surface — i.e. on an
 * `/office` route, which is reached via the footer "Office" button.
 *
 * Office-specific sidebar sections (Inbox, Projects, Agents) gate on this rather
 * than just the `office` feature flag, so they stay hidden in the regular
 * workspace and only appear once the user enters Office.
 */
export function useInOffice(): boolean {
  const officeEnabled = useFeature("office");
  const pathname = usePathname();
  return officeEnabled && !!pathname && (pathname === "/office" || pathname.startsWith("/office/"));
}
