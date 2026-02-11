// orchestrator.mock.ts - Mock Orchestrator for testing
//
// This file provides mock implementations of the Orchestrator's gRPC service
// for integration testing without requiring a real orchestrator instance.
//
// Copyright 2026 baaaht project

import { EventEmitter } from 'events';
import type { Client } from '@grpc/grpc-js';
import type {
  RegisterRequest,
  RegisterResponse,
  UnregisterRequest,
  UnregisterResponse,
  HeartbeatRequest,
  HeartbeatResponse,
  StreamAgentRequest,
  StreamAgentResponse,
  StreamTaskRequest,
  StreamTaskResponse,
  AgentMessage,
  AgentState,
  AgentType,
  MessageType,
  TaskState,
} from '../../src/proto/agent.js';

/**
 * MockOrchestratorConfig holds configuration for the mock orchestrator
 */
export interface MockOrchestratorConfig {
  /**
   * Whether registration should succeed
   * Defaults to true
   */
  registrationSucceeds?: boolean;

  /**
   * Whether heartbeat should succeed
   * Defaults to true
   */
  heartbeatSucceeds?: boolean;

  /**
   * Agent ID to return on registration
   * Defaults to a generated ID
   */
  agentId?: string;

  /**
   * Delay in milliseconds before responding
   * Useful for testing timeout behavior
   */
  responseDelay?: number;

  /**
   * Whether to simulate connection errors
   */
  simulateConnectionError?: boolean;
}

/**
 * Information about a registered agent
 */
interface AgentInfo {
  id: string;
  name: string;
  type: AgentType;
  registeredAt: Date;
  lastHeartbeat: Date;
}

/**
 * MockOrchestratorClient provides a mock gRPC client for testing
 *
 * This mock implements the same interface as OrchestratorClient but
 * returns simulated responses without connecting to a real orchestrator.
 */
export class MockOrchestratorClient extends EventEmitter {
  private config: Required<MockOrchestratorConfig>;
  private connected: boolean = false;
  private registeredAgents: Map<string, AgentInfo>;
  private agentCounter: number = 0;

  constructor(config: MockOrchestratorConfig = {}) {
    super();
    this.config = {
      registrationSucceeds: config.registrationSucceeds ?? true,
      heartbeatSucceeds: config.heartbeatSucceeds ?? true,
      agentId: config.agentId ?? '',
      responseDelay: config.responseDelay ?? 0,
      simulateConnectionError: config.simulateConnectionError ?? false,
    };
    this.registeredAgents = new Map();
  }

  /**
   * Simulates connecting to the orchestrator
   */
  async connect(): Promise<void> {
    if (this.config.simulateConnectionError) {
      throw new Error('Failed to connect to orchestrator: Connection refused');
    }

    if (this.connected) {
      throw new Error('Already connected to orchestrator');
    }

    await this.delay();
    this.connected = true;
    this.emit('connected');
  }

  /**
   * Simulates disconnecting from the orchestrator
   */
  disconnect(): void {
    this.connected = false;
    this.registeredAgents.clear();
    this.emit('disconnected');
  }

  /**
   * Checks if the client is connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Simulates agent registration
   */
  async register(request: RegisterRequest): Promise<RegisterResponse> {
    this.ensureConnected();

    await this.delay();

    if (!this.config.registrationSucceeds) {
      throw new Error('Registration failed: Agent rejected by orchestrator');
    }

    if (!request.name) {
      throw new Error('Registration failed: Missing agent name');
    }

    // Generate agent ID
    const agentId = this.config.agentId || `mock-agent-${++this.agentCounter}`;

    // Store agent info
    const agentInfo: AgentInfo = {
      id: agentId,
      name: request.name,
      type: request.type ?? AgentType.AGENT_TYPE_WORKER,
      registeredAt: new Date(),
      lastHeartbeat: new Date(),
    };
    this.registeredAgents.set(agentId, agentInfo);

    const response: RegisterResponse = {
      agentId,
      agent: {
        id: agentId,
        name: request.name,
        type: agentInfo.type,
        state: AgentState.AGENT_STATE_IDLE,
        registeredAt: new Date(),
        lastHeartbeat: new Date(),
        metadata: request.metadata,
        capabilities: request.capabilities,
      },
      registrationToken: `mock-token-${agentId}`,
      message: 'Registration successful',
    };

    this.emit('registered', { agentId, request });
    return response;
  }

  /**
   * Simulates agent unregistration
   */
  async unregister(request: UnregisterRequest): Promise<UnregisterResponse> {
    this.ensureConnected();

    await this.delay();

    const { agentId } = request;
    if (!agentId) {
      throw new Error('Unregistration failed: Missing agent ID');
    }

    const agentInfo = this.registeredAgents.get(agentId);
    if (!agentInfo) {
      throw new Error('Unregistration failed: Agent not found');
    }

    this.registeredAgents.delete(agentId);

    const response: UnregisterResponse = {
      success: true,
      message: `Agent ${agentId} unregistered successfully`,
    };

    this.emit('unregistered', { agentId });
    return response;
  }

  /**
   * Simulates sending a heartbeat
   */
  async heartbeat(request: HeartbeatRequest): Promise<HeartbeatResponse> {
    this.ensureConnected();

    await this.delay();

    if (!this.config.heartbeatSucceeds) {
      throw new Error('Heartbeat failed: Orchestrator unavailable');
    }

    const { agentId } = request;
    if (!agentId) {
      throw new Error('Heartbeat failed: Missing agent ID');
    }

    const agentInfo = this.registeredAgents.get(agentId);
    if (!agentInfo) {
      throw new Error('Heartbeat failed: Agent not registered');
    }

    // Update last heartbeat
    agentInfo.lastHeartbeat = new Date();

    const response: HeartbeatResponse = {
      timestamp: new Date(),
      state: AgentState.AGENT_STATE_IDLE,
      message: 'Heartbeat acknowledged',
    };

    this.emit('heartbeat', { agentId });
    return response;
  }

  /**
   * Gets the underlying mock gRPC client (returns a mock object)
   */
  getRawClient(): Client {
    this.ensureConnected();

    // Return a mock client object with stream methods
    const mockClient = {
      streamAgent: (() => this.createMockAgentStream()) as any,
      streamTask: (() => this.createMockTaskStream()) as any,
    } as unknown;

    return mockClient as Client;
  }

  /**
   * Gets the number of registered agents
   */
  getRegisteredAgentCount(): number {
    return this.registeredAgents.size;
  }

  /**
   * Gets info about a registered agent
   */
  getAgentInfo(agentId: string): AgentInfo | undefined {
    return this.registeredAgents.get(agentId);
  }

  /**
   * Simulates an agent message being sent from the orchestrator
   */
  async simulateIncomingMessage(agentId: string, message: AgentMessage): Promise<void> {
    const agentInfo = this.registeredAgents.get(agentId);
    if (!agentInfo) {
      throw new Error(`Agent ${agentId} not found`);
    }

    // Emit the message as if it came from the orchestrator
    this.emit('message', { agentId, message });
  }

  /**
   * Sets whether registration should succeed
   */
  setRegistrationSucceeds(value: boolean): void {
    this.config.registrationSucceeds = value;
  }

  /**
   * Sets whether heartbeat should succeed
   */
  setHeartbeatSucceeds(value: boolean): void {
    this.config.heartbeatSucceeds = value;
  }

  /**
   * Simulates a connection error on next operation
   */
  simulateError(): void {
    this.config.simulateConnectionError = true;
  }

  /**
   * Resets the mock to initial state
   */
  reset(): void {
    this.connected = false;
    this.registeredAgents.clear();
    this.agentCounter = 0;
    this.config.registrationSucceeds = true;
    this.config.heartbeatSucceeds = true;
    this.config.simulateConnectionError = false;
    this.removeAllListeners();
  }

  /**
   * Ensures the client is connected
   */
  private ensureConnected(): void {
    if (!this.connected) {
      throw new Error('Not connected to orchestrator');
    }
  }

  /**
   * Delays execution if configured
   */
  private async delay(): Promise<void> {
    if (this.config.responseDelay > 0) {
      await new Promise((resolve) => setTimeout(resolve, this.config.responseDelay));
    }
  }

  /**
   * Creates a mock task stream for StreamTask RPC
   */
  private createMockTaskStream(): EventEmitter {
    const stream = new EventEmitter();

    stream.write = () => {};
    stream.end = () => {};
    stream.cancel = () => {};

    return stream;
  }

  /**
   * Creates a mock agent stream for StreamAgent RPC
   */
  private createMockAgentStream(): EventEmitter {
    const stream = new EventEmitter();

    stream.write = () => {};
    stream.end = () => {};
    stream.cancel = () => {};

    return stream;
  }
}

/**
 * MockLLMGatewayClient provides a mock LLM Gateway client for testing
 */
export class MockLLMGatewayClient {
  private config: {
    streamingSucceeds: boolean;
    completeSucceeds: boolean;
    responseDelay: number;
  };

  private streamingEnabled: boolean = false;

  constructor(config: {
    streamingSucceeds?: boolean;
    completeSucceeds?: boolean;
    responseDelay?: number;
  } = {}) {
    this.config = {
      streamingSucceeds: config.streamingSucceeds ?? true,
      completeSucceeds: config.completeSucceeds ?? true,
      responseDelay: config.responseDelay ?? 0,
    };
  }

  /**
   * Simulates a non-streaming completion request
   */
  async complete(params: unknown): Promise<{
    content: string;
    toolCalls?: unknown[];
    usage?: unknown;
    finishReason?: string;
  }> {
    await this.delay();

    if (!this.config.completeSucceeds) {
      throw new Error('LLM Gateway error: Request failed');
    }

    return {
      content: 'This is a mock response from the LLM Gateway',
      usage: {
        promptTokens: 10,
        completionTokens: 20,
        totalTokens: 30,
      },
      finishReason: 'stop',
    };
  }

  /**
   * Simulates a streaming completion request
   */
  async *stream(params: unknown): AsyncIterableIterator<{
    type: 'content' | 'toolCall' | 'usage' | 'complete' | 'error';
    data?: unknown;
  }> {
    this.streamingEnabled = true;
    await this.delay();

    if (!this.config.streamingSucceeds) {
      yield {
        type: 'error',
        data: { message: 'LLM Gateway error: Stream failed' },
      };
      return;
    }

    // Simulate streaming response
    const chunks = ['This ', 'is ', 'a ', 'mock ', 'response'];
    for (const chunk of chunks) {
      await this.delay(10);
      yield {
        type: 'content',
        data: { content: chunk },
      };
    }

    yield {
      type: 'usage',
      data: {
        promptTokens: 10,
        completionTokens: 20,
        totalTokens: 30,
      },
    };

    yield {
      type: 'complete',
      data: { finishReason: 'stop' },
    };
  }

  /**
   * Simulates a health check
   */
  async healthCheck(): Promise<{ healthy: boolean }> {
    return { healthy: true };
  }

  /**
   * Sets whether streaming should succeed
   */
  setStreamingSucceeds(value: boolean): void {
    this.config.streamingSucceeds = value;
  }

  /**
   * Sets whether completion should succeed
   */
  setCompleteSucceeds(value: boolean): void {
    this.config.completeSucceeds = value;
  }

  /**
   * Checks if streaming is enabled
   */
  isStreamingEnabled(): boolean {
    return this.streamingEnabled;
  }

  /**
   * Resets the mock to initial state
   */
  reset(): void {
    this.config.streamingSucceeds = true;
    this.config.completeSucceeds = true;
    this.streamingEnabled = false;
  }

  /**
   * Delays execution if configured
   */
  private async delay(ms?: number): Promise<void> {
    const delay = ms ?? this.config.responseDelay;
    if (delay > 0) {
      await new Promise((resolve) => setTimeout(resolve, delay));
    }
  }
}

/**
 * Creates a new MockOrchestratorClient
 */
export function createMockOrchestratorClient(
  config?: MockOrchestratorConfig
): MockOrchestratorClient {
  return new MockOrchestratorClient(config);
}

/**
 * Creates a new MockLLMGatewayClient
 */
export function createMockLLMClient(config?: {
  streamingSucceeds?: boolean;
  completeSucceeds?: boolean;
  responseDelay?: number;
}): MockLLMGatewayClient {
  return new MockLLMGatewayClient(config);
}
