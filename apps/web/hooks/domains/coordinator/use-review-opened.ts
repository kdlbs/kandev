"use client";

import { useCallback, useEffect, useState } from "react";

const keyOf = (coordinatorId: string, actionClass: string) =>
  `kandev.coordinator.classReviewOpened.${coordinatorId}.${actionClass}`;

function readFlag(key: string): boolean {
  try {
    return window.sessionStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

/**
 * Whether the manager opened the filtered log for one class in this browser
 * session. Opening it navigates away from the settings page, so the flag is
 * kept in sessionStorage rather than component state.
 */
export function useReviewOpened(coordinatorId: string, actionClass: string): [boolean, () => void] {
  const key = keyOf(coordinatorId, actionClass);
  const [opened, setOpened] = useState(false);

  useEffect(() => {
    setOpened(readFlag(key));
  }, [key]);

  const mark = useCallback(() => {
    try {
      window.sessionStorage.setItem(key, "1");
    } catch {
      // storage unavailable: the flag still holds for this mount
    }
    setOpened(true);
  }, [key]);

  return [opened, mark];
}
