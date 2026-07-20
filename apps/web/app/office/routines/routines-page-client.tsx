"use client";

import type { Routine } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";

type RoutinesPageClientProps = {
  initialRoutines?: Routine[];
};

export function RoutinesPageClient({ initialRoutines }: RoutinesPageClientProps) {
  return <RoutinesContent initialRoutines={initialRoutines} />;
}
