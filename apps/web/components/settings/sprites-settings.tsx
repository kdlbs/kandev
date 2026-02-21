"use client";

import { useState, useCallback } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@kandev/ui/table";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import {
  IconTrash,
  IconTestPipe,
  IconLoader2,
  IconCheck,
  IconX,
  IconSparkles,
} from "@tabler/icons-react";
import { useSprites } from "@/hooks/domains/settings/use-sprites";
import { useAppStore } from "@/components/state-provider";
import {
  testSpritesConnection,
  destroySprite,
  destroyAllSprites,
} from "@/lib/api/domains/sprites-api";
import type { SpritesTestResult, SpritesTestStep } from "@/lib/types/http-sprites";

function ConnectionCard() {
  const { status } = useSprites();
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SpritesTestResult | null>(null);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testSpritesConnection();
      setTestResult(result);
    } catch {
      setTestResult({
        success: false,
        steps: [],
        total_duration_ms: 0,
        sprite_name: "",
        error: "Failed to connect to backend",
      });
    } finally {
      setTesting(false);
    }
  }, []);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <IconSparkles className="h-5 w-5" />
              Connection
            </CardTitle>
            <CardDescription>
              Sprites.dev provides ephemeral cloud sandboxes for running agents remotely.
            </CardDescription>
          </div>
          <TokenBadge configured={status?.token_configured ?? false} connected={status?.connected ?? false} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="text-sm text-muted-foreground">
          {status?.token_configured ? (
            <p>
              API token is configured via the <code className="text-xs">SPRITES_API_TOKEN</code> secret.
              {status.connected
                ? ` ${status.instance_count} active sprite${status.instance_count !== 1 ? "s" : ""}.`
                : " Unable to connect."}
            </p>
          ) : (
            <p>
              Add a secret with env key <code className="text-xs">SPRITES_API_TOKEN</code> in{" "}
              <a href="/settings/general/secrets" className="text-primary underline cursor-pointer">
                Secrets
              </a>{" "}
              to enable Sprites.dev integration.
            </p>
          )}
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={handleTest}
            disabled={testing || !status?.token_configured}
            className="cursor-pointer"
          >
            {testing ? (
              <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
            ) : (
              <IconTestPipe className="mr-1.5 h-4 w-4" />
            )}
            Test Connection
          </Button>
        </div>
        {testResult && <TestResultDisplay result={testResult} />}
      </CardContent>
    </Card>
  );
}

function TokenBadge({ configured, connected }: { configured: boolean; connected: boolean }) {
  if (!configured) {
    return <Badge variant="secondary">Not Configured</Badge>;
  }
  if (connected) {
    return <Badge variant="default" className="bg-green-600">Connected</Badge>;
  }
  return <Badge variant="destructive">Disconnected</Badge>;
}

function TestResultDisplay({ result }: { result: SpritesTestResult }) {
  return (
    <div className="rounded-md border p-3 space-y-2">
      <div className="flex items-center gap-2 text-sm font-medium">
        {result.success ? (
          <IconCheck className="h-4 w-4 text-green-600" />
        ) : (
          <IconX className="h-4 w-4 text-red-600" />
        )}
        {result.success ? "Connection test passed" : "Connection test failed"}
        <span className="text-muted-foreground font-normal">
          ({result.total_duration_ms}ms)
        </span>
      </div>
      {result.steps.map((step: SpritesTestStep) => (
        <StepRow key={step.name} step={step} />
      ))}
      {result.error && !result.steps.some((s) => s.error) && (
        <p className="text-sm text-red-600">{result.error}</p>
      )}
    </div>
  );
}

function StepRow({ step }: { step: SpritesTestStep }) {
  return (
    <div className="flex items-center gap-2 text-sm pl-2">
      {step.success ? (
        <IconCheck className="h-3 w-3 text-green-600 shrink-0" />
      ) : (
        <IconX className="h-3 w-3 text-red-600 shrink-0" />
      )}
      <span>{step.name}</span>
      <span className="text-muted-foreground">({step.duration_ms}ms)</span>
      {step.error && <span className="text-red-600 truncate">{step.error}</span>}
    </div>
  );
}

function InstancesCard() {
  const { instances, loading } = useSprites();
  const removeSpritesInstance = useAppStore((state) => state.removeSpritesInstance);
  const [destroying, setDestroying] = useState<string | null>(null);
  const [destroyingAll, setDestroyingAll] = useState(false);

  const handleDestroy = useCallback(
    async (name: string) => {
      setDestroying(name);
      try {
        await destroySprite(name);
        removeSpritesInstance(name);
      } finally {
        setDestroying(null);
      }
    },
    [removeSpritesInstance],
  );

  const handleDestroyAll = useCallback(async () => {
    setDestroyingAll(true);
    try {
      await destroyAllSprites();
      for (const inst of instances) {
        removeSpritesInstance(inst.name);
      }
    } finally {
      setDestroyingAll(false);
    }
  }, [instances, removeSpritesInstance]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Running Sprites</CardTitle>
            <CardDescription>
              Active Kandev sprites. Sprites are destroyed when agents stop.
            </CardDescription>
          </div>
          {instances.length > 0 && (
            <Button
              variant="destructive"
              size="sm"
              onClick={handleDestroyAll}
              disabled={destroyingAll}
              className="cursor-pointer"
            >
              {destroyingAll ? (
                <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
              ) : (
                <IconTrash className="mr-1.5 h-4 w-4" />
              )}
              Destroy All
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground py-4">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            Loading...
          </div>
        ) : instances.length === 0 ? (
          <p className="text-sm text-muted-foreground py-4">No active sprites.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Health</TableHead>
                <TableHead>Uptime</TableHead>
                <TableHead className="w-[80px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {instances.map((inst) => (
                <TableRow key={inst.name}>
                  <TableCell className="font-mono text-sm">{inst.name}</TableCell>
                  <TableCell>
                    <HealthBadge status={inst.health_status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatUptime(inst.uptime_seconds)}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDestroy(inst.name)}
                      disabled={destroying === inst.name}
                      className="cursor-pointer"
                    >
                      {destroying === inst.name ? (
                        <IconLoader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <IconTrash className="h-4 w-4" />
                      )}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function HealthBadge({ status }: { status: string }) {
  switch (status) {
    case "healthy":
      return <Badge variant="default" className="bg-green-600">Healthy</Badge>;
    case "unhealthy":
      return <Badge variant="destructive">Unhealthy</Badge>;
    default:
      return <Badge variant="secondary">Unknown</Badge>;
  }
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const mins = Math.floor(seconds / 60);
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  const remainMins = mins % 60;
  return `${hours}h ${remainMins}m`;
}

export function SpritesSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Sprites.dev</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage Sprites.dev remote sandbox integration for running agents in isolated cloud environments.
        </p>
      </div>
      <Separator />
      <div className="space-y-6">
        <ConnectionCard />
        <InstancesCard />
      </div>
    </div>
  );
}
