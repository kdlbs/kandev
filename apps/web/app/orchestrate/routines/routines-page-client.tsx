"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Routine } from "@/lib/state/slices/orchestrate/types";
import { RoutinesContent } from "./routines-content";

type RoutinesPageClientProps = {
  initialRoutines: Routine[];
};

export function RoutinesPageClient({ initialRoutines }: RoutinesPageClientProps) {
  const setRoutines = useAppStore((s) => s.setRoutines);

  useEffect(() => {
    if (initialRoutines.length > 0) {
      setRoutines(initialRoutines);
    }
  }, [initialRoutines, setRoutines]);

  return <RoutinesContent />;
}
