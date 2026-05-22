import { Skeleton } from "@kandev/ui/skeleton";
import {
  ActivitySkeleton,
  ChartsSkeleton,
  OverviewCardsSkeleton,
  RepositoriesSkeleton,
} from "./stats-skeletons";

export default function StatsLoading() {
  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <header className="flex items-center gap-3 p-4 pb-3 shrink-0">
        <Skeleton className="h-8 w-16" />
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-4 w-4" />
          <Skeleton className="h-4 w-48" />
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Skeleton className="h-7 w-48" />
          <Skeleton className="h-7 w-24" />
        </div>
      </header>
      <div className="flex-1 overflow-auto">
        <div className="max-w-7xl mx-auto p-6">
          <div className="space-y-5">
            <OverviewCardsSkeleton />
            <Skeleton className="h-4 w-32" />
            <ChartsSkeleton />
            <ActivitySkeleton />
            <RepositoriesSkeleton />
          </div>
        </div>
      </div>
    </div>
  );
}
