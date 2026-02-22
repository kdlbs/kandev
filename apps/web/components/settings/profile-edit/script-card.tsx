"use client";

import { useRef, useEffect, useState } from "react";
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
  const containerRef = useRef<HTMLDivElement>(null);
  const [editorHeight, setEditorHeight] = useState(height);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setEditorHeight(`${entry.contentRect.height}px`);
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div
          ref={containerRef}
          className="overflow-hidden rounded-md border resize-y"
          style={{ height, minHeight: "120px", maxHeight: "80vh" }}
        >
          <ScriptEditor
            value={value}
            onChange={onChange}
            height={editorHeight}
            placeholders={placeholders}
            executorType={executorType}
          />
        </div>
      </CardContent>
    </Card>
  );
}
