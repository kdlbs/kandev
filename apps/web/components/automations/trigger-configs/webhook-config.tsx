"use client";

import { useState, useMemo, useCallback, useEffect } from "react";
import { toast } from "sonner";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { IconCopy, IconCheck, IconEye, IconEyeOff } from "@tabler/icons-react";
import { revealWebhookSecret } from "@/lib/api/domains/automation-api";

type WebhookConfigProps = {
  automationId: string | null;
  workspaceId: string;
};

// extractPaths walks a parsed JSON object two levels deep and returns the
// dot-paths to leaf scalars (string/number/bool). For nested objects it
// returns the children's full paths so the badges suggest
// `{{webhook.pull_request.number}}` rather than `{{webhook.pull_request}}`
// (which resolves to the entire blob). Arrays surface as their root path
// only — indexing across every element would explode the badge count.
const MAX_DEPTH = 2;
function isScalar(v: unknown): boolean {
  return typeof v === "string" || typeof v === "number" || typeof v === "boolean";
}
function walkPaths(value: unknown, prefix: string, depth: number, out: string[]) {
  if (isScalar(value) || value === null) {
    out.push(prefix);
    return;
  }
  if (Array.isArray(value)) {
    // Stop at the array root — listing every index isn't useful for badges.
    out.push(prefix);
    return;
  }
  if (typeof value !== "object") return;
  if (depth >= MAX_DEPTH) {
    out.push(prefix);
    return;
  }
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    walkPaths(v, prefix ? `${prefix}.${k}` : k, depth + 1, out);
  }
}
function extractKeys(json: string): string[] {
  try {
    const parsed = JSON.parse(json);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return [];
    }
    const out: string[] = [];
    walkPaths(parsed, "", 0, out);
    return out;
  } catch {
    return [];
  }
}

export function WebhookConfig({ automationId, workspaceId }: WebhookConfigProps) {
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
        workspaceId={workspaceId}
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

type SecretState = { status: "loading" } | { status: "ready"; value: string } | { status: "error" };

function inputValueFor(
  status: SecretState["status"],
  secret: string | null,
  hidden: boolean,
  masked: string,
): string {
  if (status === "loading") return "Loading…";
  if (!secret || hidden) return masked;
  return secret;
}

function SecretField({
  automationId,
  workspaceId,
  copied,
  onCopy,
}: {
  automationId: string;
  workspaceId: string;
  copied: boolean;
  onCopy: (value: string) => void;
}) {
  // The secret is revealable any time — fetch it once on mount so users
  // landing on a saved automation can see/copy it without an extra click.
  // Combining status + value in one state avoids a setState(true) inside
  // the effect body (which the React linter flags as a cascading render).
  const [state, setState] = useState<SecretState>({ status: "loading" });
  const [hidden, setHidden] = useState(true);

  useEffect(() => {
    let cancelled = false;
    revealWebhookSecret(automationId, workspaceId)
      .then((result) => {
        if (cancelled) return;
        setState({ status: "ready", value: result.webhook_secret });
      })
      .catch((err) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : String(err);
        toast.error(`Failed to load webhook secret: ${msg}`);
        setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, [automationId, workspaceId]);

  const secret = state.status === "ready" ? state.value : null;
  const masked = "•".repeat(32);
  const inputValue = inputValueFor(state.status, secret, hidden, masked);

  return (
    <div className="space-y-1.5">
      <Label className="text-xs">Webhook secret</Label>
      <div className="flex gap-2">
        <Input
          value={inputValue}
          readOnly
          className="font-mono text-xs"
          data-testid="automation-webhook-secret-input"
        />
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer shrink-0"
          onClick={() => secret && onCopy(secret)}
          disabled={!secret}
        >
          {copied ? <IconCheck className="h-3.5 w-3.5" /> : <IconCopy className="h-3.5 w-3.5" />}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer shrink-0"
          onClick={() => setHidden((v) => !v)}
          disabled={!secret}
          data-testid="automation-webhook-secret-toggle"
        >
          {hidden ? <IconEye className="h-3.5 w-3.5" /> : <IconEyeOff className="h-3.5 w-3.5" />}
        </Button>
      </div>
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
