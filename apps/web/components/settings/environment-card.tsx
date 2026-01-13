'use client';

import Link from 'next/link';
import { IconServer, IconChevronRight } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Badge } from '@kandev/ui/badge';
import type { Environment } from '@/lib/types/http';

type EnvironmentCardProps = {
  environment: Environment;
};

export function EnvironmentCard({ environment }: EnvironmentCardProps) {
  const typeLabel = environment.kind === 'local_pc' ? 'Local' : 'Custom Image';
  const imageLabel = environment.image_tag || 'Unbuilt';

  return (
    <Link href={`/settings/environment/${environment.id}`}>
      <Card className="hover:bg-accent transition-colors cursor-pointer">
        <CardContent className="py-4">
          <div className="flex items-start justify-between">
            <div className="flex items-start gap-3 flex-1">
              <div className="p-2 bg-muted rounded-md">
                <IconServer className="h-4 w-4" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <h4 className="font-medium">{environment.name}</h4>
                  <Badge variant="secondary" className="text-xs">
                    {typeLabel}
                  </Badge>
                  {environment.kind === 'docker_image' && (
                    <Badge variant="outline" className="text-xs">
                      {imageLabel}
                    </Badge>
                  )}
                </div>
                {environment.kind === 'local_pc' && environment.worktree_root && (
                  <div className="text-xs text-muted-foreground mt-1">
                    Worktree: {environment.worktree_root}
                  </div>
                )}
              </div>
            </div>
            <IconChevronRight className="h-5 w-5 text-muted-foreground" />
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
