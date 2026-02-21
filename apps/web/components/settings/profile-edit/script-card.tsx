"use client";

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";

type ScriptCardProps = {
  title: string;
  description: string;
  value: string;
  onChange: (v: string) => void;
  height?: string;
  placeholders: ScriptPlaceholder[];
  executorType: string;
};

export function ScriptCard({
  title,
  description,
  value,
  onChange,
  height = "300px",
  placeholders,
  executorType,
}: ScriptCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="overflow-hidden rounded-md border">
          <ScriptEditor
            value={value}
            onChange={onChange}
            height={height}
            placeholders={placeholders}
            executorType={executorType}
          />
        </div>
      </CardContent>
    </Card>
  );
}
