"use client";

// Trust-confirmation block shown under the SSH test result. Disabled when the
// caller's last test ran against a different form, to keep stale fingerprints
// from being trusted for a new target.
export function FingerprintTrustBlock({
  fingerprint,
  currentlyPinned,
  trust,
  resultStale,
  onTrustChange,
}: {
  fingerprint: string;
  currentlyPinned?: string;
  trust: boolean;
  resultStale: boolean;
  onTrustChange: (v: boolean) => void;
}) {
  const fingerprintChanged = !!currentlyPinned && currentlyPinned !== fingerprint;
  return (
    <div className="mt-3 space-y-2">
      <div className="text-xs">
        <span className="text-muted-foreground">Host fingerprint observed: </span>
        <code data-testid="ssh-fingerprint-observed" className="font-mono">
          {fingerprint}
        </code>
      </div>
      {fingerprintChanged && (
        <p data-testid="ssh-fingerprint-change-warning" className="text-xs text-amber-600">
          Warning: this fingerprint differs from the one currently pinned (
          <code className="font-mono">{currentlyPinned}</code>). Trusting it overwrites the pinned
          key. If you didn’t expect a host re-key, stop here.
        </p>
      )}
      {resultStale && (
        <p data-testid="ssh-test-result-stale" className="text-xs text-amber-600">
          You changed a connection field after this test ran. Run Test Connection again before
          trusting and saving.
        </p>
      )}
      <label
        className={
          "flex items-start gap-2 text-sm " +
          (resultStale ? "cursor-not-allowed text-muted-foreground" : "cursor-pointer")
        }
      >
        <input
          type="checkbox"
          data-testid="ssh-trust-checkbox"
          checked={trust}
          disabled={resultStale}
          onChange={(e) => onTrustChange(e.target.checked)}
          className={"mt-0.5 " + (resultStale ? "cursor-not-allowed" : "cursor-pointer")}
        />
        <span>
          <strong>Trust this host.</strong> I’ve verified the fingerprint above matches my server.
          Future connections that report a different fingerprint will be refused.
        </span>
      </label>
    </div>
  );
}
