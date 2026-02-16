import { Card, CardContent, CardHeader } from '@kandev/ui/card';
import { Skeleton } from '@kandev/ui/skeleton';

export default function StatsLoading() {
  return (
    <div className="h-screen w-full flex flex-col bg-background">
      {/* Header */}
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

      {/* Content */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-7xl mx-auto p-6">
          <div className="space-y-5">
            {/* Overview Cards */}
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
              {[...Array(4)].map((_, i) => (
                <Card key={i} className="rounded-sm">
                  <CardHeader className="pb-2">
                    <Skeleton className="h-4 w-24" />
                  </CardHeader>
                  <CardContent>
                    <Skeleton className="h-9 w-16 mb-2" />
                    <Skeleton className="h-4 w-32 mb-3" />
                    <Skeleton className="h-1.5 w-full" />
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Telemetry Section */}
            <Skeleton className="h-4 w-32" />

            {/* Completed Tasks and Productivity */}
            <div className="grid gap-4 lg:grid-cols-3">
              <Card className="rounded-sm lg:col-span-2">
                <CardHeader className="pb-2">
                  <Skeleton className="h-4 w-48" />
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    <div className="flex gap-2">
                      {[...Array(3)].map((_, i) => (
                        <Skeleton key={i} className="h-7 w-16" />
                      ))}
                    </div>
                    <Skeleton className="h-32 w-full" />
                    <div className="flex justify-between">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-3 w-16" />
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card className="rounded-sm">
                <CardHeader className="pb-2">
                  <Skeleton className="h-4 w-32" />
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {[...Array(3)].map((_, i) => (
                      <div key={i} className="flex justify-between">
                        <Skeleton className="h-4 w-24" />
                        <Skeleton className="h-4 w-16" />
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Activity and Agents */}
            <div className="grid gap-4 lg:grid-cols-2">
              <Card className="rounded-sm">
                <CardHeader className="pb-2">
                  <Skeleton className="h-4 w-32" />
                </CardHeader>
                <CardContent>
                  <div className="space-y-2">
                    <Skeleton className="h-4 w-48 mb-2" />
                    <Skeleton className="h-24 w-full" />
                    <div className="flex items-center gap-2 mt-2">
                      <Skeleton className="h-3 w-8" />
                      {[...Array(5)].map((_, i) => (
                        <Skeleton key={i} className="h-2 w-2" />
                      ))}
                      <Skeleton className="h-3 w-8" />
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card className="rounded-sm">
                <CardHeader className="pb-2">
                  <Skeleton className="h-4 w-24" />
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {[...Array(3)].map((_, i) => (
                      <div key={i}>
                        <div className="flex justify-between mb-1">
                          <Skeleton className="h-4 w-32" />
                          <Skeleton className="h-4 w-8" />
                        </div>
                        <Skeleton className="h-1.5 w-full" />
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Repositories */}
            <Card className="rounded-sm">
              <CardHeader className="pb-2">
                <Skeleton className="h-4 w-40" />
              </CardHeader>
              <CardContent>
                <div className="grid gap-3 md:grid-cols-2">
                  {[...Array(4)].map((_, i) => (
                    <div key={i} className="rounded-sm border bg-muted/20 p-3">
                      <div className="flex justify-between mb-2">
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-4 w-16" />
                      </div>
                      <div className="flex gap-3 mb-3">
                        <Skeleton className="h-3 w-16" />
                        <Skeleton className="h-3 w-20" />
                        <Skeleton className="h-3 w-16" />
                      </div>
                      <Skeleton className="h-3 w-full mb-2" />
                      <Skeleton className="h-3 w-full" />
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
