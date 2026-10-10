"use client";

import { useEffect, useState } from "react";

const TICK_MS = 30_000;

/**
 * One 30-second interval per mounted screen, re-evaluating `now` so item and
 * row ages advance while the screen is open (Adoption decision 9,
 * AC-COORDINATOR-NEEDS-YOU-003.3).
 */
export function useNowTick(): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), TICK_MS);
    return () => clearInterval(id);
  }, []);

  return now;
}
