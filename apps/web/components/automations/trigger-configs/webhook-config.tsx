"use client";

import { useState, useMemo, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { IconCopy, IconCheck } from "@tabler/icons-react";

type WebhookConfigProps = {
  automationId: string | null;
};

function extractKeys(json: string): string[] {
  try {
    const parsed = JSON.parse(json);
    if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
      return Object.keys(parsed);
    }
  } catch {
    // ignore parse errors
  }
  return [];
}

export function WebhookConfig({ automationId }: WebhookConfigProps) {
  const [copied, setCopied] = useState<"url" | null>(null);
  const [samplePayload, setSamplePayload] = useState("");

  const copyToClipboard = useCallback(async (text: string) => {
    await navigator.clipboard.writeText(text);
    setCopied("url");
    setTimeout(() => setCopied(null), 2000);
  }, []);

  const detectedKeys = useMemo(() => extractKeys(samplePayload), [samplePayload]);

  if (!automationId) {
    return (
      <div className="space-y-3">
        <p className="text-xs text-muted-foreground">
          Webhook URL will be available after saving the automation.
        </p>
        <SamplePayloadSection
          samplePayload={samplePayload}
          onChange={setSamplePayload}
          detectedKeys={detectedKeys}
        />
      </div>
    );
  }

  const webhookUrl =
    typeof window !== "undefined"
      ? `${window.location.origin}/api/v1/automations/webhook/${automationId}`
      : `/api/v1/automations/webhook/${automationId}`;

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label className="text-xs">Webhook URL</Label>
        <div className="flex gap-2">
          <Input value={webhookUrl} readOnly className="font-mono text-xs" />
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer shrink-0"
            onClick={() => copyToClipboard(webhookUrl)}
          >
            {copied === "url" ? (
              <IconCheck className="h-3.5 w-3.5" />
            ) : (
              <IconCopy className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          Send a POST request with JSON body. Auth via{" "}
          <code className="bg-muted px-1 rounded">X-Webhook-Secret</code> header or{" "}
          <code className="bg-muted px-1 rounded">?secret=</code> query parameter.
        </p>
      </div>
      <SamplePayloadSection
        samplePayload={samplePayload}
        onChange={setSamplePayload}
        detectedKeys={detectedKeys}
      />
    </div>
  );
}

function SamplePayloadSection({
  samplePayload,
  onChange,
  detectedKeys,
}: {
  samplePayload: string;
  onChange: (value: string) => void;
  detectedKeys: string[];
}) {
  return (
    <div className="space-y-2">
      <div className="space-y-1.5">
        <Label className="text-xs">Sample payload (optional)</Label>
        <Textarea
          value={samplePayload}
          onChange={(e) => onChange(e.target.value)}
          placeholder='{"repo": "org/app", "env": "prod"}'
          className="font-mono text-xs min-h-[60px] resize-y"
          rows={2}
        />
        <p className="text-xs text-muted-foreground">
          Paste an example JSON body to discover available placeholders.
        </p>
      </div>
      <div className="space-y-1">
        <Label className="text-xs">Available placeholders</Label>
        <div className="flex flex-wrap gap-1.5">
          <PlaceholderBadge value="webhook.body" />
          {detectedKeys.map((key) => (
            <PlaceholderBadge key={key} value={`data.${key}`} />
          ))}
          {detectedKeys.length === 0 && !samplePayload && <PlaceholderBadge value="data.*" />}
        </div>
      </div>
    </div>
  );
}

function PlaceholderBadge({ value }: { value: string }) {
  return (
    <code className="bg-muted px-1.5 py-0.5 rounded text-xs font-mono text-muted-foreground">
      {`{{${value}}}`}
    </code>
  );
}
