"use client";

import { useCallback } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";

function PolicyRuleRow({
  rule,
  index,
  onUpdate,
  onRemove,
}: {
  rule: NetworkPolicyRule;
  index: number;
  onUpdate: (index: number, field: keyof NetworkPolicyRule, val: string) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <TableRow>
      <TableCell>
        <Input
          value={rule.domain}
          onChange={(e) => onUpdate(index, "domain", e.target.value)}
          placeholder="*.example.com"
          className="text-sm"
        />
      </TableCell>
      <TableCell>
        <Select value={rule.action} onValueChange={(v) => onUpdate(index, "action", v)}>
          <SelectTrigger className="text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="allow">
              <Badge variant="default" className="bg-green-600">
                Allow
              </Badge>
            </SelectItem>
            <SelectItem value="deny">
              <Badge variant="destructive">Deny</Badge>
            </SelectItem>
          </SelectContent>
        </Select>
      </TableCell>
      <TableCell>
        <Input
          value={rule.include ?? ""}
          onChange={(e) => onUpdate(index, "include", e.target.value)}
          placeholder="Optional pattern"
          className="text-sm"
        />
      </TableCell>
      <TableCell>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => onRemove(index)}
          className="cursor-pointer"
        >
          <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </TableCell>
    </TableRow>
  );
}

function PolicyRulesTable({
  rules,
  onUpdate,
  onRemove,
}: {
  rules: NetworkPolicyRule[];
  onUpdate: (index: number, field: keyof NetworkPolicyRule, val: string) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Domain</TableHead>
          <TableHead className="w-[120px]">Action</TableHead>
          <TableHead>Include</TableHead>
          <TableHead className="w-[60px]" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {rules.map((rule, idx) => (
          <PolicyRuleRow
            key={idx}
            rule={rule}
            index={idx}
            onUpdate={onUpdate}
            onRemove={onRemove}
          />
        ))}
      </TableBody>
    </Table>
  );
}

type NetworkPoliciesCardProps = {
  rules: NetworkPolicyRule[];
  onRulesChange: (rules: NetworkPolicyRule[]) => void;
};

export function NetworkPoliciesCard({ rules, onRulesChange }: NetworkPoliciesCardProps) {
  const addRule = useCallback(() => {
    onRulesChange([...rules, { domain: "", action: "allow" }]);
  }, [rules, onRulesChange]);

  const removeRule = useCallback(
    (index: number) => {
      onRulesChange(rules.filter((_, i) => i !== index));
    },
    [rules, onRulesChange],
  );

  const updateRule = useCallback(
    (index: number, field: keyof NetworkPolicyRule, val: string) => {
      onRulesChange(rules.map((rule, i) => (i === index ? { ...rule, [field]: val } : rule)));
    },
    [rules, onRulesChange],
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Network Policies</CardTitle>
            <CardDescription>
              Define network access rules applied when a sprite is created for this profile.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={addRule}
            className="cursor-pointer"
          >
            <IconPlus className="mr-1 h-3.5 w-3.5" />
            Add Rule
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">No network policy rules configured.</p>
        ) : (
          <PolicyRulesTable rules={rules} onUpdate={updateRule} onRemove={removeRule} />
        )}
      </CardContent>
    </Card>
  );
}
