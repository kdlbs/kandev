"use client";

import { createKnownValuesStore } from "@/lib/known-values-store";

export const {
  recordForKey,
  reset: resetKnownReposStore,
  useKnownValues: useKnownRepos,
  getSnapshot: __getSnapshotForTests,
} = createKnownValuesStore();
