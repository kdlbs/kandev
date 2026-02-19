"use client";

import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";

export function WorkspaceNotFoundCard({ onBack }: { onBack: () => void }) {
  return (
    <div>
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">Workspace not found</p>
          <Button className="mt-4" onClick={onBack}>
            Back to Workspaces
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
