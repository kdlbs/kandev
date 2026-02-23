"use client";

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type GitIdentityCardProps = {
  name: string;
  email: string;
  onNameChange: (value: string) => void;
  onEmailChange: (value: string) => void;
};

export function GitIdentityCard({
  name,
  email,
  onNameChange,
  onEmailChange,
}: GitIdentityCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Git Identity</CardTitle>
        <CardDescription>
          Optional author identity applied in remote executor environments.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="git-user-name">Git User Name</Label>
          <Input
            id="git-user-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder="Jane Developer"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="git-user-email">Git User Email</Label>
          <Input
            id="git-user-email"
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
            placeholder="jane@example.com"
          />
        </div>
      </CardContent>
    </Card>
  );
}
