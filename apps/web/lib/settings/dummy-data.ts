import type { SettingsData } from './types';

export const SETTINGS_DATA: SettingsData = {
  general: {
    theme: 'dark',
    editor: 'vscode',
    customEditorCommand: '',
    notifications: {
      taskUpdates: true,
      agentCompletion: true,
      errors: true,
    },
  },
  workspaces: [
    {
      id: '1',
      name: 'KanDev Project',
      repositories: [
        {
          id: '1',
          name: 'bismarck',
          path: '/Users/username/projects/bismarck',
          setupScript: '#!/bin/bash\nnpm install && npm run build',
          cleanupScript: '#!/bin/bash\nrm -rf node_modules dist',
          customScripts: [
            { id: 's1', name: 'dev', command: '#!/bin/bash\nnpm run dev' },
            { id: 's2', name: 'test', command: '#!/bin/bash\nnpm test' },
          ],
        },
        {
          id: '2',
          name: 'backend-api',
          path: '/Users/username/projects/backend-api',
          setupScript: '#!/bin/bash\ngo mod download',
          cleanupScript: '#!/bin/bash\ngo clean',
          customScripts: [
            { id: 's3', name: 'run', command: '#!/bin/bash\ngo run main.go' },
          ],
        },
      ],
      contexts: [
        {
          id: '1',
          name: 'Architect',
          columns: [
            { id: 'backlog', title: 'Backlog', color: 'bg-neutral-400' },
            { id: 'high-level-design', title: 'High Level Design', color: 'bg-cyan-500' },
            { id: 'low-level-design', title: 'Low Level Design', color: 'bg-violet-500' },
            { id: 'review', title: 'Review', color: 'bg-yellow-500' },
            { id: 'done', title: 'Done', color: 'bg-green-500' },
          ],
        },
        {
          id: '2',
          name: 'Dev',
          columns: [
            { id: 'todo', title: 'To Do', color: 'bg-neutral-400' },
            { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
            { id: 'review', title: 'Review', color: 'bg-yellow-500' },
            { id: 'done', title: 'Done', color: 'bg-green-500' },
          ],
        },
        {
          id: '3',
          name: 'Team',
          columns: [
            { id: 'backlog', title: 'Backlog', color: 'bg-neutral-400' },
            { id: 'solution-design', title: 'Solution Design', color: 'bg-sky-500' },
            { id: 'ready-for-dev', title: 'Ready for Dev', color: 'bg-indigo-500' },
            { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
            { id: 'review', title: 'Review', color: 'bg-yellow-500' },
            { id: 'done', title: 'Done', color: 'bg-green-500' },
          ],
        },
      ],
    },
  ],
  environments: [
    {
      id: '1',
      name: 'Development',
      type: 'local-docker',
      baseDocker: 'universal',
      envVariables: [
        { id: 'e1', key: 'NODE_ENV', value: 'development' },
        { id: 'e2', key: 'PORT', value: '3000' },
      ],
      secrets: [
        { id: 'se1', key: 'DATABASE_PASSWORD', value: '••••••••' },
        { id: 'se2', key: 'API_SECRET', value: '••••••••' },
      ],
      setupScript: '#!/bin/bash\napt-get update\napt-get install -y git curl',
      installedAgents: ['claude-code', 'codex'],
    },
    {
      id: '2',
      name: 'Production',
      type: 'remote-docker',
      baseDocker: 'golang',
      envVariables: [
        { id: 'e3', key: 'NODE_ENV', value: 'production' },
      ],
      secrets: [],
      setupScript: '',
      installedAgents: ['auggie'],
    },
  ],
  agents: [
    {
      id: '1',
      agent: 'claude-code',
      name: 'Default Profile',
      model: 'claude-sonnet-4.5',
      autoApprove: false,
      temperature: 0.7,
    },
    {
      id: '2',
      agent: 'codex',
      name: 'Fast Development',
      model: 'codex-v2',
      autoApprove: true,
      temperature: 0.5,
    },
    {
      id: '3',
      agent: 'auggie',
      name: 'Code Review',
      model: 'auggie-v1',
      autoApprove: false,
      temperature: 0.3,
    },
  ],
};
