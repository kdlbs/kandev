import type { SidebarTaskPageResponse } from "@/lib/types/http";
import type { SidebarPageRecoveryCycle } from "./sidebar-task-page-recovery";

export type ReadAttempt = {
  controller: AbortController;
  owner: string;
  readId?: string;
  released: boolean;
  settled: boolean;
};

export type RecoveryRecord = {
  cacheKey: string;
  consumers: number;
  exhausted: boolean;
  exhaustedAt?: number;
  lastError?: unknown;
  promise?: Promise<SidebarTaskPageResponse>;
  cycle?: SidebarPageRecoveryCycle<SidebarTaskPageResponse>;
  attempt?: ReadAttempt;
  retainedOwner?: string;
  lastForegroundEpisode?: object;
};
