"use client";

import { useState, useCallback } from "react";
import { IconTestPipe, IconLoader2, IconCheck, IconX, IconSparkles } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { testSpritesConnection } from "@/lib/api/domains/sprites-api";
import { useSprites } from "@/hooks/domains/settings/use-sprites";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { SpritesTestResult, SpritesTestStep } from "@/lib/types/http-sprites";

type SpritesApiKeyCardProps = {
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
};

const NONE_VALUE = "__none__";

export function SpritesApiKeyCard({ secretId, onSecretIdChange, secrets }: SpritesApiKeyCardProps) {
  const { status } = useSprites(secretId ?? undefined);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<SpritesTestResult | null>(null);

  const handleTest = useCallback(async () => {
    if (!secretId) return;
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
        error: "Failed to connect to backend",
      });
    } finally {
      setTesting(false);
    }
  }, [secretId]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <IconSparkles className="h-5 w-5" />
              API Key
            </CardTitle>
            <CardDescription>
              Select the secret containing your Sprites.dev API token.
            </CardDescription>
          </div>
          <ConnectionBadge secretId={secretId} status={status} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label>Secret</Label>
          <Select
            value={secretId ?? NONE_VALUE}
            onValueChange={(v) => onSecretIdChange(v === NONE_VALUE ? null : v)}
          >
            <SelectTrigger className="cursor-pointer">
              <SelectValue placeholder="Select a secret..." />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE_VALUE}>None</SelectItem>
              {secrets.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {secretId && (
          <ConnectionDetails
            status={status}
            testing={testing}
            testResult={testResult}
            onTest={handleTest}
          />
        )}
      </CardContent>
    </Card>
  );
}

function ConnectionBadge({
  secretId,
  status,
}: {
  secretId: string | null;
  status: ReturnType<typeof useSprites>["status"];
}) {
  if (!secretId) {
    return <Badge variant="secondary">Not Configured</Badge>;
  }
  if (status?.connected) {
    return (
      <Badge variant="default" className="bg-green-600">
        Connected
      </Badge>
    );
  }
  if (status?.token_configured) {
    return <Badge variant="destructive">Disconnected</Badge>;
  }
  return <Badge variant="secondary">Checking...</Badge>;
}

function ConnectionDetails({
  status,
  testing,
  testResult,
  onTest,
}: {
  status: ReturnType<typeof useSprites>["status"];
  testing: boolean;
  testResult: SpritesTestResult | null;
  onTest: () => void;
}) {
  return (
    <>
      <ConnectionStatusText status={status} />
      <Button
        variant="outline"
        size="sm"
        onClick={onTest}
        disabled={testing}
        className="cursor-pointer"
      >
        {testing ? (
          <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
        ) : (
          <IconTestPipe className="mr-1.5 h-4 w-4" />
        )}
        Test Connection
      </Button>
      {testResult && <TestResultDisplay result={testResult} />}
    </>
  );
}

function ConnectionStatusText({ status }: { status: ReturnType<typeof useSprites>["status"] }) {
  if (status?.connected) {
    const count = status.instance_count;
    return (
      <p className="text-sm text-muted-foreground">
        Connected. {count} active sprite{count !== 1 ? "s" : ""}.
      </p>
    );
  }
  if (status?.token_configured) {
    return <p className="text-sm text-muted-foreground">Token configured but unable to connect.</p>;
  }
  return <p className="text-sm text-muted-foreground">Verifying connection...</p>;
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
