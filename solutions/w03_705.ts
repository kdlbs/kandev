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
import crypto from 'crypto';

const ALGORITHM = 'aes-256-gcm';
const KEY_LENGTH = 32;
const IV_LENGTH = 16;

export class CredentialEncryption {
  private key: Buffer;

  constructor(workspaceId: string) {
    // Derive key from workspace-specific secret
    const secret = process.env.JIRA_ENCRYPTION_SECRET || 'default-secret-change-in-production';
    this.key = crypto.scryptSync(secret, workspaceId, KEY_LENGTH);
  }

  encrypt(credentials: JiraConfig): string {
    const iv = crypto.randomBytes(IV_LENGTH);
    const cipher = crypto.createCipheriv(ALGORITHM, this.key, iv);
    
    let encrypted = cipher.update(JSON.stringify(credentials), 'utf8', 'hex');
    encrypted += cipher.final('hex');
    
    const authTag = cipher.getAuthTag().toString('hex');
    
    return `${iv.toString('hex')}:${authTag}:${encrypted}`;
  }

  decrypt(encryptedData: string): JiraConfig {
    const [ivHex, authTagHex, encrypted] = encryptedData.split(':');
    
    const decipher = crypto.createDecipheriv(
      ALGORITHM, 
      this.key, 
      Buffer.from(ivHex, 'hex')
    );
    
    decipher.setAuthTag(Buffer.from(authTagHex, 'hex'));
    
    let decrypted = decipher.update(encrypted, 'hex', 'utf8');
    decrypted += decipher.final('utf8');
    
    return JSON.parse(decrypted);
  }
}

// src/integrations/jira/api.ts
import axios, { AxiosInstance } from 'axios';

export class JiraClient {
  private client: AxiosInstance;
  private config: JiraConfig;

  constructor(config: JiraConfig) {
    this.config = config;
    const auth = Buffer.from(`${config.email}:${config.apiToken}`).toString('base64');
    
    this.client = axios.create({
      baseURL: `${config.siteUrl}/rest/api/3`,
      headers: {
        'Authorization': `Basic ${auth}`,
        'Content-Type': 'application/json',
      },
    });
  }

  async testConnection(): Promise<boolean> {
    try {
      await this.client.get('/myself');
      return true;
    } catch {
      return false;
    }
  }

  async searchTickets(jql: string, startAt = 0, maxResults = 50): Promise<JiraSearchResult> {
    const response = await this.client.post('/search', {
      jql,
      startAt,
      maxResults,
      fields: ['summary', 'status', 'assignee', 'priority', 'created', 'updated'],
    });

    return {
      tickets: response.data.issues.map(this.mapIssue),
      total: response.data.total,
      startAt: response.data.startAt,
      maxResults: response.data.maxResults,
    };
  }

  async getTicket(ticketKey: string): Promise<JiraTicket> {
    const response = await this.client.get(`/issue/${ticketKey}`, {
      params: {
        fields: ['summary', 'status', 'assignee', 'priority', 'created', 'updated'],
      },
    });

    return this.mapIssue(response.data);
  }

  private mapIssue(issue: any): JiraTicket {
    return {
      id: issue.id,
      key: issue.key,
      summary: issue.fields.summary,
      status: issue.fields.status.name,
      assignee: issue.fields.assignee?.displayName || 'Unassigned',
      priority: issue.fields.priority?.name || 'None',
      created: issue.fields.created,
      updated: issue.fields.updated,
    };
  }
}

// src/integrations/jira/poller.ts
import { EventEmitter } from 'events';

export class JiraHealthPoller extends EventEmitter {
  private interval: NodeJS.Timeout | null = null;
  private client: JiraClient | null = null;
  private readonly POLL_INTERVAL = 90000; // 90 seconds

  start(client: JiraClient): void {
    this.client = client;
    this.poll();
    this.interval = setInterval(() => this.poll(), this.POLL_INTERVAL);
  }

  stop(): void {
    if (this.interval) {
      clearInterval(this.interval);
      this.interval = null;
    }
    this.client = null;
  }

  async immediateProbe(): Promise<void> {
    await this.poll();
  }

  private async poll(): Promise<void> {
    if (!this.client) return;

    try {
      const healthy = await this.client.testConnection();
      this.emit('health', healthy);
    } catch {
      this.emit('health', false);
    }
  }
}

// src/integrations/jira/store.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { CredentialEncryption } from './encryption';

interface JiraStore {
  config: JiraConfig | null;
  isConnected: boolean;
  savedViews: Array<{ name: string; jql: string }>;
  taskPresets: Array<{ name: string; project: string; issueType: string }>;
  
  setConfig: (config: JiraConfig) => void;
  clearConfig: () => void;
  setConnected: (connected: boolean) => void;
  addSavedView: (view: { name: string; jql: string }) => void;
  removeSavedView: (name: string) => void;
  addTaskPreset: (preset: { name: string; project: string; issueType: string }) => void;
  removeTaskPreset: (name: string) => void;
}

export const useJiraStore = create<JiraStore>()(
  persist(
    (set) => ({
      config: null,
      isConnected: false,
      savedViews: [],
      taskPresets: [],

      setConfig: (config) => {
        const encryption = new CredentialEncryption('workspace-id');
        const encrypted = encryption.encrypt(config);
        localStorage.setItem('jira-credentials', encrypted);
        set({ config });
      },

      clearConfig: () => {
        localStorage.removeItem('jira-credentials');
        set({ config: null, isConnected: false });
      },

      setConnected: (connected) => set({ isConnected: connected }),

      addSavedView: (view) =>
        set((state) => ({ savedViews: [...state.savedViews, view] })),

      removeSavedView: (name) =>
        set((state) => ({
          savedViews: state.savedViews.filter((v) => v.name !== name),
        })),

      addTaskPreset: (preset) =>
        set((state) => ({ taskPresets: [...state.taskPresets, preset] })),

      removeTaskPreset: (name) =>
        set((state) => ({
          taskPresets: state.taskPresets.filter((p) => p.name !== name),
        })),
    }),
    {
      name: 'jira-store',
      partialize: (state) => ({
        savedViews: state.savedViews,
        taskPresets: state.taskPresets,
      }),
    }
  )
);

// src/pages/jira/index.tsx
import React, { useState, useEffect, useCallback } from 'react';
import { useJiraStore } from '../../integrations/jira/store';
import { JiraClient } from '../../integrations/jira/api';
import { JiraHealthPoller } from '../../integrations/jira/poller';
import { JiraTicket } from '../../integrations/jira/types';
import { TaskCreateDialog } from '../../components/TaskCreateDialog';

const JiraPage: React.FC = () => {
  const { config, isConnected, setConnected, savedViews, taskPresets } = useJiraStore();
  const [tickets, setTickets] = useState<JiraTicket[]>([]);
  const [totalTickets, setTotalTickets] = useState(0);
  const [currentPage, setCurrentPage] = useState(0);
  const [searchQuery, setSearchQuery] = useState('');
  const [activeFilter, setActiveFilter] = useState('all');
  const [showTaskDialog, setShowTaskDialog] = useState(false);
  const [selectedTicket, setSelectedTicket] = useState<JiraTicket | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const poller = new JiraHealthPoller();

  useEffect(() => {
    if (config) {
      const client = new JiraClient(config);
      
      poller.on('health', (healthy: boolean) => {
        setConnected(healthy);
      });
      
      poller.start(client);
      poller.immediateProbe();

      return () => {
        poller.stop();
      };
    }
  }, [config]);

  const fetchTickets = useCallback(async (page = 0) => {
    if (!config) return;
    
    setLoading(true);
    setError(null);

    try {
      const client = new JiraClient(config);
      let jql = '';

      switch (activeFilter) {
        case 'my':
          jql = `assignee = currentUser()`;
          break;
        case 'recent':
          jql = 'ORDER BY updated DESC';
          break;
        case 'open':
          jql = 'status NOT IN (Closed, Resolved, Done)';
          break;
        default:
          jql = searchQuery || 'ORDER BY created DESC';
      }

      if (searchQuery && activeFilter === 'all') {
        jql = `text ~ "${searchQuery}" ORDER BY created DESC`;
      }

      const result = await client.searchTickets(jql, page * 50, 50);
      setTickets(result.tickets);
      setTotalTickets(result.total);
      setCurrentPage(page);
    } catch (err) {
      setError('Failed to fetch tickets. Please check your connection.');
    } finally {
      setLoading(false);
    }
  }, [config, activeFilter, searchQuery]);

  useEffect(() => {
    fetchTickets();
  }, [fetchTickets]);

  const handleTicketClick = (ticket: JiraTicket) => {
    setSelectedTicket(ticket);
    setShowTaskDialog(true);
  };

  const handleCreateTask = (taskData: any) => {
    // Logic to create task with linked Jira ticket
    console.log('Creating task with data:', taskData);
    setShowTaskDialog(false);
  };

  const filters = [
    { id: 'all', label: 'All Tickets' },
    { id: 'my', label: 'My Tickets' },
    { id: 'recent', label: 'Recently Updated' },
    { id: 'open', label: 'Open' },
  ];

  return (
    <div className="jira-page">
      <div className="jira-header">
        <h1>Jira Integration</h1>
        <div className="connection-status">
          <span className={`status-dot ${isConnected ? 'connected' : 'disconnected'}`} />
          {isConnected ? 'Connected' : 'Disconnected'}
        </div>
      </div>

      <div className="jira-controls">
        <div className="filter-pills">
          {filters.map((filter) => (
            <button
              key={filter.id}
              className={`filter-pill ${activeFilter === filter.id ? 'active' : ''}`}
              onClick={() => setActiveFilter(filter.id)}
            >
              {filter.label}
            </button>
          ))}
        </div>

        <div className="search-bar">
          <input
            type="text"
            placeholder="Search tickets or enter JQL..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && fetchTickets()}
          />
          <button onClick={() => fetchTickets()}>Search</button>
        </div>

        {savedViews.length > 0 && (
          <div className="saved-views">
            <select onChange={(e) => {
              const view = savedViews.find(v => v.name === e.target.value);
              if (view) {
                setSearchQuery(view.jql);
                fetchTickets();
              }
            }}>
              <option value="">Saved Views</option>
              {savedViews.map((view) => (
                <option key={view.name} value={view.name}>{view.name}</option>
              ))}
            </select>
          </div>
        )}
      </div>

      {error && <div className="error-message">{error}</div>}

      <div className="tickets-list">
        {loading ? (
          <div className="loading">Loading tickets...</div>
        ) : tickets.length === 0 ? (
          <div className="no-tickets">No tickets found</div>
        ) : (
          <>
            {tickets.map((ticket) => (
              <div
                key={ticket.id}
                className="ticket-card"
                onClick={() => handleTicketClick(ticket)}
              >
                <div className="ticket-key">{ticket.key}</div>
                <div className="ticket-summary">{ticket.summary}</div>
                <div className="ticket-meta">
                  <span className={`status status-${ticket.status.toLowerCase()}`}>
                    {ticket.status}
                  </span>
                  <span className="priority">{ticket.priority}</span>
                  <span className="assignee">{ticket.assignee}</span>
                </div>
              </div>
            ))}

            <div className="pagination">
              <button
                disabled={currentPage === 0}
                onClick={() => fetchTickets(currentPage - 1)}
              >
                Previous
              </button>
              <span>
                Page {currentPage + 1} of {Math.ceil(totalTickets / 50)}
              </span>
              <button
                disabled={(currentPage + 1) * 50 >= totalTickets}
                onClick={() => fetchTickets(currentPage + 1)}
              >
                Next
              </button>
            </div>
          </>
        )}
      </div>

      {showTaskDialog && selectedTicket && (
        <TaskCreateDialog
          ticket={selectedTicket}
          presets={taskPresets}
          onClose={() => setShowTaskDialog(false)}
          onCreate={handleCreateTask}
        />
      )}
    </div>
  );
};

export default JiraPage;
