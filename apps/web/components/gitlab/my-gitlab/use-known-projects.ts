"use client";

import { createKnownValuesStore } from "@/lib/known-values-store";

export const {
  recordForKey,
  reset: resetKnownProjectsStore,
  useKnownValues: useKnownProjects,
  getSnapshot: __getSnapshotForTests,
} = createKnownValuesStore();
