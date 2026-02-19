"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";

export default function AgentEditPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/settings/agents");
  }, [router]);

  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-sm text-muted-foreground">Manage agents from the main Agents page.</p>
        <Button className="mt-4" onClick={() => router.push("/settings/agents")}>
          Go to Agents
        </Button>
      </CardContent>
    </Card>
  );
}
