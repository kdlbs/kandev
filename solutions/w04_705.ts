// src/integrations/jira/types.ts
export interface JiraConfig {
  siteUrl: string;
  email: string;
  apiToken: string;
  defaultProject: string;
}

export interface JiraTicket {
  id: string;
  key: string;
  summary: string;
  status: string;
  assignee: string;
  priority: string;
  created: string;
  updated: string;
}

export interface JiraSearchResult {
  tickets: JiraTicket[];
  total: number;
  startAt: number;
  maxResults: number;
}

// src/integrations/jira/encryption.ts
import { createCipheriv, createDecipheriv, randomBytes } from 'crypto';

const ALGORITHM = 'aes-256-gcm';
const KEY = process.env.ENCRYPTION_KEY || randomBytes(32);

export function encryptCredentials(credentials: JiraConfig): string {
  const iv = randomBytes(16);
  const cipher = createCipheriv(ALGORITHM, KEY, iv);
  
  let encrypted = cipher.update(JSON.stringify(credentials), 'utf8', 'hex');
  encrypted += cipher.final('hex');
  
  const authTag = cipher.getAuthTag().toString('hex');
  
  return JSON.stringify({
    iv: iv.toString('hex'),
    encrypted,
    authTag
  });
}

export function decryptCredentials(encryptedData: string): JiraConfig {
  const { iv, encrypted, authTag } = JSON.parse(encryptedData);
  
  const decipher = createDecipheriv(ALGORITHM, KEY, Buffer.from(iv, 'hex'));
  decipher.setAuthTag(Buffer.from(authTag, 'hex'));
  
  let decrypted = decipher.update(encrypted, 'hex', 'utf8');
  decrypted += decipher.final('utf8');
  
  return JSON.parse(decrypted);
}

// src/integrations/jira/api.ts
import axios, { AxiosInstance } from 'axios';
import { JiraConfig, JiraTicket, JiraSearchResult } from './types';

export class JiraClient {
  private client: AxiosInstance;
  private config: JiraConfig;

  constructor(config: JiraConfig) {
    this.config = config;
    this.client = axios.create({
      baseURL: `${config.siteUrl}/rest/api/3`,
      auth: {
        username: config.email,
        password: config.apiToken
      },
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json'
      }
    });
  }

  async checkHealth(): Promise<boolean> {
    try {
      await this.client.get('/myself');
      return true;
    } catch {
      return false;
    }
  }

  async searchTickets(jql: string, startAt: number = 0, maxResults: number = 50): Promise<JiraSearchResult> {
    const response = await this.client.post('/search', {
      jql,
      startAt,
      maxResults,
      fields: ['summary', 'status', 'assignee', 'priority', 'created', 'updated']
    });

    return {
      tickets: response.data.issues.map((issue: any) => ({
        id: issue.id,
        key: issue.key,
        summary: issue.fields.summary,
        status: issue.fields.status.name,
        assignee: issue.fields.assignee?.displayName || 'Unassigned',
        priority: issue.fields.priority?.name || 'None',
        created: issue.fields.created,
        updated: issue.fields.updated
      })),
      total: response.data.total,
      startAt: response.data.startAt,
      maxResults: response.data.maxResults
    };
  }

  async getTicket(key: string): Promise<JiraTicket> {
    const response = await this.client.get(`/issue/${key}`, {
      params: {
        fields: ['summary', 'status', 'assignee', 'priority', 'created', 'updated']
      }
    });

    return {
      id: response.data.id,
      key: response.data.key,
      summary: response.data.fields.summary,
      status: response.data.fields.status.name,
      assignee: response.data.fields.assignee?.displayName || 'Unassigned',
      priority: response.data.fields.priority?.name || 'None',
      created: response.data.fields.created,
      updated: response.data.fields.updated
    };
  }
}

// src/integrations/jira/store.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { JiraConfig, JiraTicket } from './types';
import { encryptCredentials, decryptCredentials } from './encryption';

interface JiraStore {
  config: JiraConfig | null;
  encryptedConfig: string | null;
  isConnected: boolean;
  lastHealthCheck: number | null;
  savedViews: Array<{ name: string; jql: string }>;
  defaultProject: string;
  
  setConfig: (config: JiraConfig) => void;
  clearConfig: () => void;
  setConnected: (connected: boolean) => void;
  addSavedView: (name: string, jql: string) => void;
  removeSavedView: (name: string) => void;
  setDefaultProject: (project: string) => void;
}

export const useJiraStore = create<JiraStore>()(
  persist(
    (set, get) => ({
      config: null,
      encryptedConfig: null,
      isConnected: false,
      lastHealthCheck: null,
      savedViews: [],
      defaultProject: '',
      
      setConfig: (config: JiraConfig) => {
        const encrypted = encryptCredentials(config);
        set({ config, encryptedConfig: encrypted, defaultProject: config.defaultProject });
      },
      
      clearConfig: () => {
        set({ config: null, encryptedConfig: null, isConnected: false, lastHealthCheck: null });
      },
      
      setConnected: (connected: boolean) => {
        set({ isConnected: connected, lastHealthCheck: Date.now() });
      },
      
      addSavedView: (name: string, jql: string) => {
        const { savedViews } = get();
        set({ savedViews: [...savedViews, { name, jql }] });
      },
      
      removeSavedView: (name: string) => {
        const { savedViews } = get();
        set({ savedViews: savedViews.filter(v => v.name !== name) });
      },
      
      setDefaultProject: (project: string) => {
        set({ defaultProject: project });
      }
    }),
    {
      name: 'jira-storage',
      partialize: (state) => ({
        encryptedConfig: state.encryptedConfig,
        savedViews: state.savedViews,
        defaultProject: state.defaultProject
      }),
      onRehydrateStorage: () => (state) => {
        if (state?.encryptedConfig) {
          try {
            const config = decryptCredentials(state.encryptedConfig);
            state.config = config;
          } catch {
            state.clearConfig();
          }
        }
      }
    }
  )
);

// src/integrations/jira/poller.ts
import { useEffect, useRef } from 'react';
import { useJiraStore } from './store';
import { JiraClient } from './api';

const POLL_INTERVAL = 90000; // 90 seconds

export function useJiraHealthPoller() {
  const { config, setConnected } = useJiraStore();
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const clientRef = useRef<JiraClient | null>(null);

  useEffect(() => {
    if (config) {
      clientRef.current = new JiraClient(config);
      
      const checkHealth = async () => {
        if (clientRef.current) {
          const healthy = await clientRef.current.checkHealth();
          setConnected(healthy);
        }
      };

      // Immediate check
      checkHealth();

      // Periodic check
      intervalRef.current = setInterval(checkHealth, POLL_INTERVAL);

      return () => {
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
        }
      };
    } else {
      setConnected(false);
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    }
  }, [config, setConnected]);
}

// src/pages/jira/JiraPage.tsx
import React, { useState, useEffect, useCallback } from 'react';
import { useJiraStore } from '../../integrations/jira/store';
import { useJiraHealthPoller } from '../../integrations/jira/poller';
import { JiraClient } from '../../integrations/jira/api';
import { JiraTicket, JiraSearchResult } from '../../integrations/jira/types';
import { JiraConfigDialog } from './JiraConfigDialog';
import { JiraTicketList } from './JiraTicketList';
import { JiraSearchBar } from './JiraSearchBar';
import { JiraFilterPills } from './JiraFilterPills';
import { JiraSavedViews } from './JiraSavedViews';
import { JiraTaskLinkDialog } from './JiraTaskLinkDialog';

export function JiraPage() {
  const { config, isConnected, savedViews, defaultProject } = useJiraStore();
  useJiraHealthPoller();

  const [client, setClient] = useState<JiraClient | null>(null);
  const [searchResult, setSearchResult] = useState<JiraSearchResult | null>(null);
  const [currentJql, setCurrentJql] = useState<string>('');
  const [currentPage, setCurrentPage] = useState(0);
  const [loading, setLoading] = useState(false);
  const [showConfig, setShowConfig] = useState(!config);
  const [showTaskLink, setShowTaskLink] = useState(false);
  const [selectedTicket, setSelectedTicket] = useState<JiraTicket | null>(null);
  const [activeFilters, setActiveFilters] = useState<string[]>([]);

  const pageSize = 50;

  useEffect(() => {
    if (config) {
      setClient(new JiraClient(config));
      setCurrentJql(`project = "${config.defaultProject}" ORDER BY created DESC`);
    }
  }, [config]);

  const searchTickets = useCallback(async (jql: string, startAt: number = 0) => {
    if (!client) return;
    
    setLoading(true);
    try {
      const result = await client.searchTickets(jql, startAt, pageSize);
      setSearchResult(result);
      setCurrentJql(jql);
      setCurrentPage(Math.floor(startAt / pageSize));
    } catch (error) {
      console.error('Failed to search tickets:', error);
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    if (currentJql && client) {
      searchTickets(currentJql);
    }
  }, [currentJql, client, searchTickets]);

  const handleFilterChange = (filters: string[]) => {
    setActiveFilters(filters);
    let jql = `project = "${defaultProject}"`;
    
    if (filters.length > 0) {
      const filterConditions = filters.map(f => {
        switch (f) {
          case 'open': return 'status NOT IN (Closed, Resolved, Done)';
          case 'assigned': return 'assignee IS NOT EMPTY';
          case 'unassigned': return 'assignee IS EMPTY';
          case 'high_priority': return 'priority IN (Highest, High)';
          case 'recent': return 'created >= -7d';
          default: return '';
        }
      }).filter(Boolean);
      
      if (filterConditions.length > 0) {
        jql += ` AND (${filterConditions.join(' AND ')})`;
      }
    }
    
    jql += ' ORDER BY created DESC';
    searchTickets(jql);
  };

  const handleSavedViewSelect = (jql: string) => {
    searchTickets(jql);
  };

  const handlePageChange = (page: number) => {
    if (client) {
      searchTickets(currentJql, page * pageSize);
    }
  };

  const handleTaskLink = (ticket: JiraTicket) => {
    setSelectedTicket(ticket);
    setShowTaskLink(true);
  };

  return (
    <div className="jira-page">
      <div className="jira-header">
        <h1>Jira Integration</h1>
        <div className="connection-status">
          <span className={`status-indicator ${isConnected ? 'connected' : 'disconnected'}`} />
          <span>{isConnected ? 'Connected' : 'Disconnected'}</span>
          <button onClick={() => setShowConfig(true)} className="config-button">
            Configure
          </button>
        </div>
      </div>

      {showConfig && <JiraConfigDialog onClose={() => setShowConfig(false)} />}
      {showTaskLink && selectedTicket && (
        <JiraTaskLinkDialog 
          ticket={selectedTicket} 
          onClose={() => {
            setShowTaskLink(false);
            setSelectedTicket(null);
          }}
        />
      )}

      {config && (
        <div className="jira-content">
          <div className="jira-sidebar">
            <JiraSavedViews 
              views={savedViews}
              onSelect={handleSavedViewSelect}
              currentJql={currentJql}
            />
          </div>
          
          <div className="jira-main">
            <JiraSearchBar 
              onSearch={(jql) => searchTickets(jql)}
              defaultJql={currentJql}
            />
            
            <JiraFilterPills 
              activeFilters={activeFilters}
              onFilterChange={handleFilterChange}
            />
            
            <JiraTicketList 
              searchResult={searchResult}
              loading={loading}
              currentPage={currentPage}
              pageSize={pageSize}
              onPageChange={handlePageChange}
              onTaskLink={handleTaskLink}
            />
          </div>
        </div>
      )}
    </div>
  );
}

// src/pages/jira/JiraConfigDialog.tsx
import React, { useState } from 'react';
import { useJiraStore } from '../../integrations/jira/store';
import { JiraClient } from '../../integrations/jira/api';

interface Props {
  onClose: () => void;
}

export function JiraConfigDialog({ onClose }: Props) {
  const { setConfig, clearConfig, config } = useJiraStore();
  const [siteUrl, setSiteUrl] = useState(config?.siteUrl || '');
  const [email, setEmail] = useState(config?.email || '');
  const [apiToken, setApiToken] = useState(config?.apiToken || '');
  const [defaultProject, setDefaultProject] = useState(config?.defaultProject || '');
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState('');

  const handleTestConnection = async () => {
    setTesting(true);
    setError('');
    
    try {
      const testClient = new JiraClient({ siteUrl, email, apiToken, defaultProject });
      const healthy = await testClient.checkHealth();
      
      if (healthy) {
        setConfig({ siteUrl, email, apiToken, defaultProject });
        onClose();
      } else {
        setError('Failed to connect to Jira. Please check your credentials.');
      }
    } catch (err) {
      setError('Connection failed: ' + (err as Error).message);
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="modal-overlay">
      <div className="modal-content">
        <h2>Jira Configuration</h2>
        
        <div className="form-group">
          <label>Site URL</label>
          <input
            type="url"
            value={siteUrl}
            onChange={(e) => setSiteUrl(e.target.value)}
            placeholder="https://your-domain.atlassian.net"
            required
          />
        </div>
        
        <div className="form-group">
          <label>Email</label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="your-email@example.com"
            required
          />
        </div>
        
        <div className="form-group">
          <label>API Token</label>
          <input
            type="password"
            value={apiToken}
            onChange={(e) => setApiToken(e.target.value)}
            placeholder="Your Jira API token"
            required
          />
        </div>
        
        <div className="form-group">
          <label>Default Project</label>
          <input
            type="text"
            value={defaultProject}
            onChange={(e) => setDefaultProject(e.target.value)}
            placeholder="PROJECT_KEY"
            required
          />
        </div>
        
        {error && <div className="error-message">{error}</div>}
        
        <div className="modal-actions">
          <button onClick={onClose} className="btn-secondary">
            Cancel
          </button>
          <button onClick={handleTestConnection} disabled={testing} className="btn-primary">
            {testing ? 'Testing...' : 'Test & Save'}
          </button>
        </div>
      </div>
    </div>
  );
}

// src/pages/jira/JiraTicketList.tsx
import React from 'react';
import { JiraSearchResult, JiraTicket } from '../../integrations/jira/types';

interface Props {
  searchResult: JiraSearchResult | null;
  loading: boolean;
  currentPage: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onTaskLink: (ticket: JiraTicket) => void;
}

export function JiraTicketList({ searchResult, loading, currentPage, pageSize, onPageChange, onTaskLink }: Props) {
  if (loading) {
    return <div className="loading">Loading tickets...</div>;
  }

  if (!searchResult) {
    return <div className="empty-state">No tickets found. Try adjusting your search.</div>;
  }

  const totalPages = Math.ceil(searchResult.total / pageSize);

  return (
    <div className="ticket-list">
      <div className="ticket-count">
        Showing {searchResult.tickets.length} of {searchResult.total} tickets
      </div>
      
      <table className="ticket-table">
        <thead>
          <tr>
            <th>Key</th>
            <th>Summary</th>
            <th>Status</th>
            <th>Assignee</th>
            <th>Priority</th>
            <th