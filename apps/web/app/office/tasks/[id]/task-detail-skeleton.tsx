import { Skeleton } from "@kandev/ui/skeleton";

export function IssueDetailSkeleton() {
  return (
    <div className="flex h-full">
      <div className="flex-1 min-w-0 overflow-y-auto p-6">
        {/* Breadcrumb */}
        <Skeleton className="h-4 w-48" />
        {/* Header row */}
        <div className="flex items-center gap-2 mt-4">
          <Skeleton className="h-5 w-5 rounded-full" />
          <Skeleton className="h-4 w-20" />
        </div>
        {/* Title */}
        <Skeleton className="h-6 w-80 mt-4" />
        {/* Description */}
        <Skeleton className="h-4 w-full mt-4" />
        <Skeleton className="h-4 w-3/4 mt-2" />
        {/* Buttons */}
        <div className="flex gap-2 mt-6">
          <Skeleton className="h-8 w-32" />
          <Skeleton className="h-8 w-36" />
        </div>
        {/* Tabs */}
        <div className="mt-6">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-20 w-full mt-4" />
        </div>
      </div>
      <div className="w-80 border-l border-border shrink-0 overflow-y-auto p-4">
        <Skeleton className="h-4 w-24 mb-4" />
        <Skeleton className="h-8 w-full mb-2" />
        <Skeleton className="h-8 w-full mb-2" />
        <Skeleton className="h-8 w-full mb-2" />
        <Skeleton className="h-8 w-full mb-2" />
      </div>
    </div>
  );
}
