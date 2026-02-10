// registration.ts - Agent registration and heartbeat management
//
// This file handles agent registration with the orchestrator and
// maintains the heartbeat connection to keep the agent alive.
//
// Copyright 2026 baaaht project

import { AgentType, AgentState, AgentMetadata, AgentCapabilities, ResourceLimits, ResourceUsage } from '../proto/agent.js';
import { OrchestratorClient, OrchestratorClientConfig, createOrchestratorClient } from './grpc-client.js';

/**
 * AgentRegistrationInfo holds information about the agent for registration
 */
export interface AgentRegistrationInfo {
  /**
   * The name of the agent (e.g., 'assistant', 'worker', 'researcher')
   */
  name: string;

  /**
   * The type of agent
   */
  type: AgentType;

  /**
   * Optional metadata about the agent
   */
  metadata?: AgentMetadata;

  /**
   * Optional capabilities declaration
   */
  capabilities?: AgentCapabilities;
}

/**
 * HeartbeatConfig holds configuration for the heartbeat loop
 */
export interface HeartbeatConfig {
  /**
   * Interval between heartbeats in milliseconds
   * Defaults to 5000 (5 seconds)
   */
  interval?: number;

  /**
   * Maximum missed heartbeats before considering connection lost
   * Defaults to 3
   */
  maxMissed?: number;

  /**
   * Whether to automatically restart heartbeat after failure
   * Defaults to false
   */
  autoRestart?: boolean;
}

/**
 * AgentRegistrationResult holds the result of a successful registration
 */
export interface AgentRegistrationResult {
  /**
   * The agent ID assigned by the orchestrator
   */
  agentId: string;

  /**
   * The registration token to use for heartbeats
   */
  registrationToken: string;

  /**
   * The state of the agent after registration
   */
  state: AgentState;
}

/**
 * RegisteredAgent holds the state of a registered agent
 */
export interface RegisteredAgent {
  /**
   * The gRPC client connected to the orchestrator
   */
  client: OrchestratorClient;

  /**
   * The agent ID assigned by the orchestrator
   */
  agentId: string;

  /**
   * The registration token
   */
  registrationToken: string;

  /**
   * The current state of the agent
   */
  state: AgentState;

  /**
   * Whether the heartbeat loop is running
   */
  heartbeatRunning: boolean;

  /**
   * The heartbeat interval ID
   */
  heartbeatInterval?: NodeJS.Timeout;

  /**
   * The number of consecutive failed heartbeats
   */
  missedHeartbeats: number;
}

/**
 * AgentRegistry manages agent registration and heartbeat lifecycle
 */
export class AgentRegistry {
  private registeredAgent: RegisteredAgent | null = null;
  private heartbeatConfig: Required<HeartbeatConfig>;

  constructor(heartbeatConfig: HeartbeatConfig = {}) {
    this.heartbeatConfig = {
      interval: heartbeatConfig.interval ?? 5000,
      maxMissed: heartbeatConfig.maxMissed ?? 3,
      autoRestart: heartbeatConfig.autoRestart ?? false,
    };
  }

  /**
   * Registers the agent with the orchestrator
   *
   * @param info - The agent registration information
   * @param clientConfig - Optional configuration for the orchestrator client
   * @returns The registration result with agent ID and token
   * @throws Error if registration fails or if already registered
   */
  async register(
    info: AgentRegistrationInfo,
    clientConfig: OrchestratorClientConfig = {}
  ): Promise<AgentRegistrationResult> {
    if (this.registeredAgent) {
      throw new Error('Agent is already registered. Call unregister() first.');
    }

    // Create and connect the orchestrator client
    const client = await createOrchestratorClient(clientConfig);

    try {
      // Prepare the registration request
      const request = {
        name: info.name,
        type: info.type,
        metadata: info.metadata ?? this.getDefaultMetadata(),
        capabilities: info.capabilities ?? this.getDefaultCapabilities(),
      };

      // Register with the orchestrator
      const response = await client.register(request);

      if (!response.agentId || !response.registrationToken) {
        throw new Error('Invalid registration response from orchestrator');
      }

      // Store the registered agent state
      this.registeredAgent = {
        client,
        agentId: response.agentId,
        registrationToken: response.registrationToken,
        state: AgentState.AGENT_STATE_IDLE,
        heartbeatRunning: false,
        missedHeartbeats: 0,
      };

      return {
        agentId: response.agentId,
        registrationToken: response.registrationToken,
        state: AgentState.AGENT_STATE_IDLE,
      };
    } catch (err) {
      // Clean up the client if registration fails
      client.disconnect();
      throw err;
    }
  }

  /**
   * Unregisters the agent from the orchestrator
   *
   * @param reason - Optional reason for unregistration
   * @throws Error if unregistration fails or if not registered
   */
  async unregister(reason?: string): Promise<void> {
    if (!this.registeredAgent) {
      throw new Error('Agent is not registered');
    }

    // Stop the heartbeat loop first
    this.stopHeartbeat();

    const agent = this.registeredAgent;

    try {
      // Unregister from the orchestrator
      await agent.client.unregister({
        agentId: agent.agentId,
        reason: reason ?? 'Agent initiated unregistration',
      });
    } finally {
      // Always disconnect the client
      agent.client.disconnect();
      this.registeredAgent = null;
    }
  }

  /**
   * Starts the heartbeat loop to keep the agent connection alive
   *
   * @throws Error if agent is not registered or heartbeat is already running
   */
  startHeartbeat(): void {
    if (!this.registeredAgent) {
      throw new Error('Agent is not registered');
    }

    if (this.registeredAgent.heartbeatRunning) {
      throw new Error('Heartbeat is already running');
    }

    this.registeredAgent.heartbeatRunning = true;
    this.registeredAgent.missedHeartbeats = 0;

    // Start the heartbeat loop
    this.registeredAgent.heartbeatInterval = setInterval(
      () => this.sendHeartbeat(),
      this.heartbeatConfig.interval
    );
  }

  /**
   * Stops the heartbeat loop
   */
  stopHeartbeat(): void {
    if (this.registeredAgent?.heartbeatInterval) {
      clearInterval(this.registeredAgent.heartbeatInterval);
      this.registeredAgent.heartbeatInterval = undefined;
      this.registeredAgent.heartbeatRunning = false;
    }
  }

  /**
   * Gets the registered agent information
   *
   * @returns The registered agent info or null if not registered
   */
  getRegisteredAgent(): RegisteredAgent | null {
    return this.registeredAgent;
  }

  /**
   * Gets the agent ID
   *
   * @returns The agent ID or null if not registered
   */
  getAgentId(): string | null {
    return this.registeredAgent?.agentId ?? null;
  }

  /**
   * Gets the orchestrator client
   *
   * @returns The client or null if not registered
   */
  getClient(): OrchestratorClient | null {
    return this.registeredAgent?.client ?? null;
  }

  /**
   * Checks if the agent is registered
   */
  isRegistered(): boolean {
    return this.registeredAgent !== null;
  }

  /**
   * Checks if the heartbeat loop is running
   */
  isHeartbeatRunning(): boolean {
    return this.registeredAgent?.heartbeatRunning ?? false;
  }

  /**
   * Sends a single heartbeat to the orchestrator
   *
   * @private
   */
  private async sendHeartbeat(): Promise<void> {
    if (!this.registeredAgent) {
      return;
    }

    const agent = this.registeredAgent;

    try {
      // Prepare heartbeat request with current state
      const request = {
        agentId: agent.agentId,
        resources: this.getCurrentResourceUsage(),
        activeTasks: [], // TODO: Track active tasks
      };

      // Send heartbeat
      await agent.client.heartbeat(request);

      // Reset missed heartbeat counter on success
      agent.missedHeartbeats = 0;
    } catch (err) {
      const error = err as Error;
      agent.missedHeartbeats++;

      // Log error (in production, use proper logging)
      if (agent.missedHeartbeats >= this.heartbeatConfig.maxMissed) {
        // Max missed heartbeats reached
        this.stopHeartbeat();

        if (this.heartbeatConfig.autoRestart) {
          // Attempt to reconnect and restart heartbeat
          this.attemptReconnect();
        } else {
          // Mark agent as disconnected
          agent.state = AgentState.AGENT_STATE_TERMINATED;
        }
      }
    }
  }

  /**
   * Attempts to reconnect to the orchestrator after connection loss
   *
   * @private
   */
  private async attemptReconnect(): Promise<void> {
    if (!this.registeredAgent) {
      return;
    }

    const agent = this.registeredAgent;

    try {
      // Disconnect and reconnect the client
      agent.client.disconnect();
      await agent.client.connect();

      // Re-register with the same agent ID
      const response = await agent.client.register({
        name: 'assistant', // TODO: Store original registration info
        type: AgentType.AGENT_TYPE_WORKER,
      });

      if (response.agentId && response.registrationToken) {
        agent.agentId = response.agentId;
        agent.registrationToken = response.registrationToken;
        agent.state = AgentState.AGENT_STATE_IDLE;
        agent.missedHeartbeats = 0;

        // Restart heartbeat
        this.startHeartbeat();
      }
    } catch {
      // Reconnection failed, mark as terminated
      agent.state = AgentState.AGENT_STATE_TERMINATED;
    }
  }

  /**
   * Gets default metadata for the agent
   *
   * @private
   */
  private getDefaultMetadata(): AgentMetadata {
    return {
      version: process.env.npm_package_version ?? '0.1.0',
      description: 'Assistant Agent - Primary conversational interface',
      labels: {
        environment: process.env.NODE_ENV ?? 'development',
      },
      hostname: require('os').hostname(),
      pid: process.pid.toString(),
    };
  }

  /**
   * Gets default capabilities for the agent
   *
   * @private
   */
  private getDefaultCapabilities(): AgentCapabilities {
    return {
      supportedTasks: ['conversation', 'delegation'],
      supportedTools: ['delegate'],
      maxConcurrentTasks: 10,
      resourceLimits: {
        nanoCpus: BigInt(1000000000), // 1 CPU
        memoryBytes: BigInt(1024 * 1024 * 1024), // 1GB
      },
      supportsStreaming: true,
      supportsCancellation: true,
      supportedProtocols: ['grpc', 'uds'],
    };
  }

  /**
   * Gets current resource usage for heartbeat
   *
   * @private
   */
  private getCurrentResourceUsage(): ResourceUsage {
    const usage = process.cpuUsage();
    const memoryUsage = process.memoryUsage();

    return {
      cpuUsageNs: BigInt((usage.user + usage.system) * 1000),
      memoryBytes: BigInt(memoryUsage.heapUsed),
    };
  }
}

/**
 * Creates a new AgentRegistry instance
 *
 * @param heartbeatConfig - Optional configuration for heartbeat behavior
 * @returns A new AgentRegistry instance
 */
export function createAgentRegistry(heartbeatConfig?: HeartbeatConfig): AgentRegistry {
  return new AgentRegistry(heartbeatConfig);
}
