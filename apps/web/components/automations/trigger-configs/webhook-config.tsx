"use client";

import { useState, useMemo, useCallback } from "react";
import { toast } from "sonner";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { IconCopy, IconCheck, IconEye, IconEyeOff } from "@tabler/icons-react";
import { revealWebhookSecret } from "@/lib/api/domains/automation-api";

type WebhookConfigProps = {
  automationId: string | null;
  // initialSecret is the plaintext secret returned by the create response.
  // Shown unmasked exactly once; absent on later edits, where the user
  // must click "Reveal" to fetch it from the dedicated endpoint.
  initialSecret?: string | null;
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

export function WebhookConfig({ automationId, initialSecret }: WebhookConfigProps) {
  const [copied, setCopied] = useState<"url" | "secret" | null>(null);
  const [samplePayload, setSamplePayload] = useState("");

  const copyValue = useCallback(async (value: string, kind: "url" | "secret") => {
    await navigator.clipboard.writeText(value);
    setCopied(kind);
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
      <UrlField
        url={webhookUrl}
        copied={copied === "url"}
        onCopy={() => copyValue(webhookUrl, "url")}
      />
      <SecretField
        automationId={automationId}
        initialSecret={initialSecret ?? null}
        copied={copied === "secret"}
        onCopy={(value) => copyValue(value, "secret")}
      />
      <p className="text-xs text-muted-foreground">
        Send a POST request with a JSON body and the secret in the{" "}
        <code className="bg-muted px-1 rounded">X-Webhook-Secret</code> header. Reference fields
        from the payload with <code className="bg-muted px-1 rounded">{`{{webhook.<path>}}`}</code>,
        e.g. <code className="bg-muted px-1 rounded">{`{{webhook.pull_request.number}}`}</code>.
      </p>
      <SamplePayloadSection
        samplePayload={samplePayload}
        onChange={setSamplePayload}
        detectedKeys={detectedKeys}
      />
    </div>
  );
}

function UrlField({ url, copied, onCopy }: { url: string; copied: boolean; onCopy: () => void }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">Webhook URL</Label>
      <div className="flex gap-2">
        <Input value={url} readOnly className="font-mono text-xs" />
        <Button variant="outline" size="sm" className="cursor-pointer shrink-0" onClick={onCopy}>
          {copied ? <IconCheck className="h-3.5 w-3.5" /> : <IconCopy className="h-3.5 w-3.5" />}
        </Button>
      </div>
    </div>
  );
}

function SecretField({
  automationId,
  initialSecret,
  copied,
  onCopy,
}: {
  automationId: string;
  initialSecret: string | null;
  copied: boolean;
  onCopy: (value: string) => void;
}) {
  // secret holds the plaintext value when known. Starts populated when the
  // editor just created the automation (one-time reveal); null afterwards
  // so the user has to actively click Reveal to load it.
  const [secret, setSecret] = useState<string | null>(initialSecret);
  const [revealing, setRevealing] = useState(false);

  const handleReveal = async () => {
    setRevealing(true);
    try {
      const result = await revealWebhookSecret(automationId);
      setSecret(result.webhook_secret);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`Failed to reveal secret: ${msg}`);
    } finally {
      setRevealing(false);
    }
  };

  const handleHide = () => setSecret(null);
  const masked = "•".repeat(32);
  const justCreated = initialSecret !== null && secret === initialSecret;

  return (
    <div className="space-y-1.5">
      <Label className="text-xs">Webhook secret</Label>
      <div className="flex gap-2">
        <Input
          value={secret ?? masked}
          readOnly
          className="font-mono text-xs"
          data-testid="automation-webhook-secret-input"
        />
        {secret === null ? (
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer shrink-0"
            onClick={handleReveal}
            disabled={revealing}
            data-testid="automation-webhook-secret-reveal"
          >
            <IconEye className="h-3.5 w-3.5" />
          </Button>
        ) : (
          <>
            <Button
              variant="outline"
              size="sm"
              className="cursor-pointer shrink-0"
              onClick={() => onCopy(secret)}
            >
              {copied ? (
                <IconCheck className="h-3.5 w-3.5" />
              ) : (
                <IconCopy className="h-3.5 w-3.5" />
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="cursor-pointer shrink-0"
              onClick={handleHide}
            >
              <IconEyeOff className="h-3.5 w-3.5" />
            </Button>
          </>
        )}
      </div>
      {justCreated && (
        <p className="text-xs text-amber-500">
          Copy this secret now — once you leave this page you&apos;ll need to reveal it again.
        </p>
      )}
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
            <PlaceholderBadge key={key} value={`webhook.${key}`} />
          ))}
          {detectedKeys.length === 0 && !samplePayload && (
            <PlaceholderBadge value="webhook.<path>" />
          )}
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
