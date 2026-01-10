'use client';

import { useState } from 'react';
import {
  IconBrandAws,
  IconBrandDocker,
  IconCloud,
  IconServer,
  IconKey,
  IconShield,
} from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';

type Plugin = {
  id: string;
  name: string;
  description: string;
  icon: React.ReactNode;
  installed: boolean;
  category: 'infrastructure' | 'security';
};

export default function PluginsPage() {
  const [plugins, setPlugins] = useState<Plugin[]>([
    {
      id: 'aws',
      name: 'AWS Cloud',
      description: 'Deploy and manage resources on Amazon Web Services',
      icon: <IconBrandAws className="h-5 w-5" />,
      installed: false,
      category: 'infrastructure',
    },
    {
      id: 'hetzner',
      name: 'Hetzner',
      description: 'Deploy applications to Hetzner Cloud infrastructure',
      icon: <IconServer className="h-5 w-5" />,
      installed: false,
      category: 'infrastructure',
    },
    {
      id: 'flyio',
      name: 'Fly.io',
      description: 'Deploy apps globally with Fly.io edge infrastructure',
      icon: <IconCloud className="h-5 w-5" />,
      installed: false,
      category: 'infrastructure',
    },
    {
      id: 'cloudflare',
      name: 'Cloudflare',
      description: 'Use Cloudflare Workers and edge compute platform',
      icon: <IconShield className="h-5 w-5" />,
      installed: false,
      category: 'infrastructure',
    },
    {
      id: 'docker',
      name: 'Docker',
      description: 'Container runtime for local and remote environments',
      icon: <IconBrandDocker className="h-5 w-5" />,
      installed: true,
      category: 'infrastructure',
    },
    {
      id: 'sso',
      name: 'Single Sign-On',
      description: 'Enterprise SSO authentication provider',
      icon: <IconKey className="h-5 w-5" />,
      installed: false,
      category: 'security',
    },
  ]);

  const handleToggleInstall = (id: string) => {
    setPlugins(plugins.map(plugin =>
      plugin.id === id ? { ...plugin, installed: !plugin.installed } : plugin
    ));
  };

  const groupedPlugins = {
    'Infrastructure': plugins.filter(p => p.category === 'infrastructure'),
    'Security': plugins.filter(p => p.category === 'security'),
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Plugins</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Extend functionality with infrastructure and security plugins
        </p>
      </div>

      <Separator />

      <div className="space-y-8">
        {Object.entries(groupedPlugins).map(([category, items]) => (
          <div key={category} className="space-y-4">
            <h3 className="text-lg font-semibold">{category}</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {items.map((plugin) => (
                <Card key={plugin.id}>
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-3">
                        <div className="p-2 bg-muted rounded-md">
                          {plugin.icon}
                        </div>
                        <div>
                          <CardTitle className="text-base">{plugin.name}</CardTitle>
                          {plugin.installed && (
                            <Badge variant="secondary" className="mt-1">
                              Installed
                            </Badge>
                          )}
                        </div>
                      </div>
                    </div>
                    <CardDescription className="mt-2">
                      {plugin.description}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button
                      variant={plugin.installed ? 'outline' : 'default'}
                      className="w-full"
                      onClick={() => handleToggleInstall(plugin.id)}
                    >
                      {plugin.installed ? 'Uninstall' : 'Install'}
                    </Button>
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
