'use client';

import React, { useState } from 'react';
import {
  IconBrandGithub,
  IconBrandOpenai,
  IconBrandGoogle,
  IconKey,
  IconBox,
  IconCircleDot,
} from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';

type Integration = {
  id: string;
  name: string;
  description: string;
  icon: React.ReactNode;
  connected: boolean;
  category: 'ai' | 'project-management';
};

export default function IntegrationsPage() {
  const [integrations, setIntegrations] = useState<Integration[]>([
    {
      id: 'github',
      name: 'GitHub',
      description: 'Connect your GitHub repositories for code management and CI/CD',
      icon: <IconBrandGithub className="h-5 w-5" />,
      connected: false,
      category: 'project-management',
    },
    {
      id: 'anthropic',
      name: 'Anthropic',
      description: 'Connect to Anthropic API for Claude models',
      icon: <IconBox className="h-5 w-5" />,
      connected: false,
      category: 'ai',
    },
    {
      id: 'openai',
      name: 'OpenAI',
      description: 'Integrate OpenAI models and services',
      icon: <IconBrandOpenai className="h-5 w-5" />,
      connected: false,
      category: 'ai',
    },
    {
      id: 'augment',
      name: 'Augment',
      description: 'Connect Augment AI coding assistant',
      icon: <IconBox className="h-5 w-5" />,
      connected: false,
      category: 'ai',
    },
    {
      id: 'gemini',
      name: 'Gemini',
      description: 'Google Gemini AI models integration',
      icon: <IconBrandGoogle className="h-5 w-5" />,
      connected: false,
      category: 'ai',
    },
    {
      id: 'jira',
      name: 'Jira',
      description: 'Sync tasks with Atlassian Jira',
      icon: <IconBox className="h-5 w-5" />,
      connected: false,
      category: 'project-management',
    },
    {
      id: 'linear',
      name: 'Linear',
      description: 'Integrate with Linear for issue tracking',
      icon: <IconCircleDot className="h-5 w-5" />,
      connected: false,
      category: 'project-management',
    },
  ]);

  const handleConnect = (id: string) => {
    setIntegrations(integrations.map(int =>
      int.id === id ? { ...int, connected: !int.connected } : int
    ));
  };

  const groupedIntegrations = {
    'AI Models': integrations.filter(i => i.category === 'ai'),
    'Project Management': integrations.filter(i => i.category === 'project-management'),
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Integrations</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Connect external services and tools to enhance your workflow
        </p>
      </div>

      <Separator />

      <div className="space-y-8">
        {Object.entries(groupedIntegrations).map(([category, items]) => (
          <div key={category} className="space-y-4">
            <h3 className="text-lg font-semibold">{category}</h3>
            <div className="space-y-2">
              {items.map((integration) => (
                <Card key={integration.id}>
                  <CardContent className="p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-3 flex-1">
                        <div className="p-2 bg-muted rounded-lg">
                          {React.isValidElement(integration.icon)
                            ? React.cloneElement(
                                integration.icon as React.ReactElement<{ className?: string }>,
                                { className: 'h-5 w-5' }
                              )
                            : integration.icon}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <h4 className="font-medium">{integration.name}</h4>
                            {integration.connected && (
                              <span className="text-sm text-primary">Connected</span>
                            )}
                          </div>
                          {!integration.connected && (
                            <p className="text-sm text-muted-foreground">Disconnected</p>
                          )}
                        </div>
                      </div>
                      <Button
                        variant={integration.connected ? 'outline' : 'default'}
                        onClick={() => handleConnect(integration.id)}
                      >
                        {integration.connected ? 'Disconnect' : 'Connect'}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
