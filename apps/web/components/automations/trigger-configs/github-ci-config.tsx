"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type GitHubCIConfigProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

const CI_CONCLUSIONS = [
  { value: "success", label: "Success" },
  { value: "failure", label: "Failure" },
  { value: "cancelled", label: "Cancelled" },
] as const;

export function GitHubCIConfig({ config, onUpdate }: GitHubCIConfigProps) {
  const conclusions = (config.conclusions as string[]) ?? [];
  const configCheckNames = ((config.check_names as string[]) ?? []).join(", ");
  const [checkNames, setCheckNames] = useState(configCheckNames);
  useEffect(() => {
    setCheckNames(configCheckNames);
  }, [configCheckNames]);

  const toggleConclusion = (conclusion: string) => {
    const next = conclusions.includes(conclusion)
      ? conclusions.filter((c) => c !== conclusion)
      : [...conclusions, conclusion];
    onUpdate({ ...config, conclusions: next });
  };

  const handleCheckNamesBlur = () => {
    const parsed = checkNames
      .split(",")
      .map((n) => n.trim())
      .filter(Boolean);
    onUpdate({ ...config, check_names: parsed });
  };

  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label className="text-xs">Conclusions</Label>
        <div className="flex flex-wrap gap-3">
          {CI_CONCLUSIONS.map((c) => (
            <label key={c.value} className="flex items-center gap-1.5 cursor-pointer">
              <Checkbox
                checked={conclusions.includes(c.value)}
                onCheckedChange={() => toggleConclusion(c.value)}
                className="cursor-pointer"
              />
              <span className="text-sm">{c.label}</span>
            </label>
          ))}
        </div>
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs">Check names (comma-separated, optional)</Label>
        <Input
          value={checkNames}
          onChange={(e) => setCheckNames(e.target.value)}
          onBlur={handleCheckNamesBlur}
          placeholder="build, test, lint"
        />
      </div>
    </div>
  );
}
