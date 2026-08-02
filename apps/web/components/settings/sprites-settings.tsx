"use client";

import { useState, useCallback } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
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
import type { SpritesInstance, SpritesTestResult, SpritesTestStep } from "@/lib/types/http-sprites";
import { Trans, useTranslation } from "react-i18next";

export function SpritesConnectionCard({ secretId }: { secretId?: string }) {
  const { t } = useTranslation();
  const { status } = useSprites(secretId);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SpritesTestResult | null>(null);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testSpritesConnection(secretId);
      setTestResult(result);
    } catch {
      setTestResult({
        success: false,
        steps: [],
        total_duration_ms: 0,
        sprite_name: "",
        error: t("settings:failedToConnectToBackend"),
      });
    } finally {
      setTesting(false);
    }
  }, [secretId, t]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <IconSparkles className="h-5 w-5" />
              {t("settings:connection")}
            </CardTitle>
            <CardDescription>
              {t("settings:spritesDevProvidesEphemeralCloudSandboxes")}
            </CardDescription>
          </div>
          <TokenBadge
            configured={status?.token_configured ?? false}
            connected={status?.connected ?? false}
          />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="text-sm text-muted-foreground">
          {status?.token_configured ? (
            <p>
              {t("settings:apiTokenIsConfigured")}{" "}
              {status.connected
                ? t("settings:activeSprites", { count: status.instance_count })
                : t("settings:unableToConnect")}
            </p>
          ) : (
            <p>
              <Trans i18nKey="settings:configureSpritesApiToken">
                Configure a <code className="text-xs">SPRITES_API_TOKEN</code> environment variable
                in the executor profile, referencing a secret with your Sprites.dev API token.
              </Trans>
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
            {t("settings:testConnection")}
          </Button>
        </div>
        {testResult && <TestResultDisplay result={testResult} />}
      </CardContent>
    </Card>
  );
}

function TokenBadge({ configured, connected }: { configured: boolean; connected: boolean }) {
  const { t } = useTranslation();
  if (!configured) {
    return <Badge variant="secondary">{t("settings:notConfigured")}</Badge>;
  }
  if (connected) {
    return (
      <Badge variant="default" className="bg-green-600">
        {t("settings:connected")}
      </Badge>
    );
  }
  return <Badge variant="destructive">{t("settings:disconnected")}</Badge>;
}

function TestResultDisplay({ result }: { result: SpritesTestResult }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border p-3 space-y-2">
      <div className="flex items-center gap-2 text-sm font-medium">
        {result.success ? (
          <IconCheck className="h-4 w-4 text-green-600" />
        ) : (
          <IconX className="h-4 w-4 text-red-600" />
        )}
        {result.success ? t("settings:connectionTestPassed") : t("settings:connectionTestFailed")}
        <span className="text-muted-foreground font-normal">({result.total_duration_ms}ms)</span>
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

export function SpritesInstancesCard({ secretId }: { secretId?: string }) {
  const { t } = useTranslation();
  const { instances, loading } = useSprites(secretId);
  const removeSpritesInstance = useAppStore((state) => state.removeSpritesInstance);
  const [destroying, setDestroying] = useState<string | null>(null);
  const [destroyingAll, setDestroyingAll] = useState(false);

  const handleDestroy = useCallback(
    async (name: string) => {
      setDestroying(name);
      try {
        await destroySprite(name, secretId);
        removeSpritesInstance(name);
      } finally {
        setDestroying(null);
      }
    },
    [secretId, removeSpritesInstance],
  );

  const handleDestroyAll = useCallback(async () => {
    setDestroyingAll(true);
    try {
      await destroyAllSprites(secretId);
      for (const inst of instances) {
        removeSpritesInstance(inst.name);
      }
    } finally {
      setDestroyingAll(false);
    }
  }, [secretId, instances, removeSpritesInstance]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>{t("settings:runningSprites")}</CardTitle>
            <CardDescription>
              {t("settings:activeKandevSpritesSpritesAreDestroyed")}
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
              {t("settings:destroyAll")}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <InstancesContent
          loading={loading}
          instances={instances}
          destroying={destroying}
          onDestroy={handleDestroy}
        />
      </CardContent>
    </Card>
  );
}

function InstancesContent({
  loading,
  instances,
  destroying,
  onDestroy,
}: {
  loading: boolean;
  instances: SpritesInstance[];
  destroying: string | null;
  onDestroy: (name: string) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground py-4">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        {t("settings:loading")}
      </div>
    );
  }
  if (instances.length === 0) {
    return <p className="text-sm text-muted-foreground py-4">{t("settings:noActiveSprites")}</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("settings:name")}</TableHead>
          <TableHead>{t("settings:health")}</TableHead>
          <TableHead>{t("settings:uptime")}</TableHead>
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
                onClick={() => onDestroy(inst.name)}
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
  );
}

/**
 * Health status arrives from the backend as a sentinel and is switched on below,
 * so it is never translated — only the rendered label is, via a per-status key.
 *
 * The backend lower-cases whatever Sprites.dev reports and does not constrain it
 * (`NormalizeSpriteStatus`), so a status outside this map is possible; those keep
 * the previous capitalize-the-sentinel rendering rather than showing raw
 * lowercase.
 */
const HEALTH_STATUS_LABEL_KEYS: Record<string, string> = {
  running: "settings:spriteHealthRunning",
  cold: "settings:spriteHealthCold",
  starting: "settings:spriteHealthStarting",
  stopped: "settings:spriteHealthStopped",
  stopping: "settings:spriteHealthStopping",
  failed: "settings:spriteHealthFailed",
  healthy: "settings:spriteHealthHealthy",
  unhealthy: "settings:spriteHealthUnhealthy",
  unknown: "settings:spriteHealthUnknown",
};

function HealthBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const labelKey = HEALTH_STATUS_LABEL_KEYS[status];
  const label = labelKey ? t(labelKey) : status.charAt(0).toUpperCase() + status.slice(1);
  switch (status) {
    case "running":
      return (
        <Badge variant="default" className="bg-green-600">
          {label}
        </Badge>
      );
    case "cold":
      return (
        <Badge variant="secondary" className="bg-blue-600 text-white">
          {label}
        </Badge>
      );
    case "starting":
      return <Badge variant="secondary">{label}</Badge>;
    case "stopped":
    case "stopping":
    case "failed":
      return <Badge variant="destructive">{label}</Badge>;
    default:
      return <Badge variant="secondary">{label}</Badge>;
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
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div>
        {/* Brand noun — not translatable copy (docs/i18n.md, "Do not translate"). */}
        <h2 className="text-2xl font-bold">Sprites.dev</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("settings:manageSpritesDevRemoteSandboxIntegration")}
        </p>
      </div>
      <Separator />
      <div className="space-y-6">
        <SpritesConnectionCard />
        <SpritesInstancesCard />
      </div>
    </div>
  );
}
