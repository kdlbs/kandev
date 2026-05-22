"use client";

import { useEffect, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type GitHubPushConfigProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

export function GitHubPushConfig({ config, onUpdate }: GitHubPushConfigProps) {
  const configBranches = ((config.branches as string[]) ?? []).join(", ");
  const [branches, setBranches] = useState(configBranches);
  useEffect(() => {
    setBranches(configBranches);
  }, [configBranches]);

  const handleBlur = () => {
    const parsed = branches
      .split(",")
      .map((b) => b.trim())
      .filter(Boolean);
    onUpdate({ ...config, branches: parsed });
  };

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label className="text-xs">Branch patterns (comma-separated, supports globs)</Label>
        <Input
          value={branches}
          onChange={(e) => setBranches(e.target.value)}
          onBlur={handleBlur}
          placeholder="main, release/*"
        />
        <p className="text-xs text-muted-foreground">
          Triggers when code is pushed to matching branches
        </p>
      </div>
    </div>
  );
}
