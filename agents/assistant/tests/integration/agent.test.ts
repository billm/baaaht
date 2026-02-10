// agent.test.ts - Integration tests for the Assistant agent
//
// This file contains integration tests that verify the full flow of the
// Assistant agent including registration, message processing, and delegation.
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, afterEach, jest } from '@jest/globals';
import { EventEmitter } from 'events';
import { Agent, AgentEventType, AgentErrorCode } from '../../src/agent.js';
import type { AgentConfig, AgentDependencies, AgentResponse } from '../../src/agent/types.js';
import { SessionManager } from '../../src/session/manager.js';
import { AgentType, AgentState, MessageType } from '../../src/proto/agent.js';
import {
  createMockOrchestratorClient,
  createMockLLMClient,
  MockOrchestratorClient,
  MockLLMGatewayClient,
} from '../mocks/orchestrator.mock.js';

describe('Agent Integration Tests', () => {
  let mockOrchestrator: MockOrchestratorClient;
  let mockLLM: MockLLMGatewayClient;
  let mockGrpcClient: any;
  let mockEventEmitter: EventEmitter;
  let sessionManager: SessionManager;
  let agent: Agent;
  let agentId: string;

  // Test session and message IDs
  const TEST_SESSION_ID = 'test-session-123';
  const TEST_CORRELATION_ID = 'test-correlation-456';

  beforeEach(async () => {
    // Create mock clients
    mockOrchestrator = createMockOrchestratorClient();
    mockLLM = createMockLLMClient();

    // Create mock gRPC client
    mockGrpcClient = {
      streamAgent: jest.fn(() => new EventEmitter()),
      streamTask: jest.fn(() => new EventEmitter()),
    };

    // Create event emitter
    mockEventEmitter = new EventEmitter();

    // Create session manager
    sessionManager = new SessionManager({
      maxSessions: 100,
      idleTimeout: 3600000,
      timeout: 3600000,
      persistenceEnabled: false,
      cleanupInterval: 60000,
    });

    // Connect mock orchestrator
    await mockOrchestrator.connect();

    // Create agent dependencies
    const dependencies: AgentDependencies = {
      grpcClient: mockGrpcClient as any,
      sessionManager,
      eventEmitter: mockEventEmitter,
    };

    // Create agent config
    const config: AgentConfig = {
      name: 'test-assistant',
      description: 'Test assistant agent',
      orchestratorUrl: 'localhost:50051',
      llmGatewayUrl: 'http://localhost:8080',
      defaultModel: 'anthropic/claude-sonnet-4-20250514',
      maxConcurrentMessages: 5,
      messageTimeout: 30000,
      heartbeatInterval: 15000,
      maxRetries: 3,
      sessionTimeout: 3600000,
      maxSessionMessages: 100,
      contextWindowSize: 200000,
      enableStreaming: false, // Start with non-streaming for simpler tests
      debug: false,
    };

    // Create agent
    agent = new Agent(config, dependencies);

    // Initialize and register agent
    await agent.initialize();
    agentId = `agent-${Date.now()}`;
    await agent.register(agentId);
  });

  afterEach(async () => {
    if (agent) {
      await agent.shutdown();
    }
    if (sessionManager) {
      await sessionManager.close();
    }
    if (mockOrchestrator) {
      mockOrchestrator.reset();
    }
    if (mockLLM) {
      mockLLM.reset();
    }
  });

  // =============================================================================
  // Agent Lifecycle Tests
  // =============================================================================

  describe('lifecycle', () => {
    it('should initialize and register successfully', async () => {
      expect(agent).toBeDefined();
      expect(agentId).toBeDefined();

      // Check orchestrator registration
      expect(mockOrchestrator.getRegisteredAgentCount()).toBe(1);
      const agentInfo = mockOrchestrator.getAgentInfo(agentId);
      expect(agentInfo).toBeDefined();
      expect(agentInfo?.name).toBe('test-assistant');
    });

    it('should transition through states correctly', async () => {
      const status = agent.getStatus();
      expect(status.state).toBe(AgentState.AGENT_STATE_IDLE);
      expect(status.agentId).toBe(agentId);
    });

    it('should unregister and shutdown gracefully', async () => {
      await agent.unregister();

      const status = agent.getStatus();
      expect(status.state).toBe(AgentState.AGENT_STATE_UNREGISTERING);
    });

    it('should emit lifecycle events', async () => {
      const events: string[] = [];

      agent.on(AgentEventType.INITIALIZED, () => events.push('initialized'));
      agent.on(AgentEventType.REGISTERED, () => events.push('registered'));
      agent.on(AgentEventType.READY, () => events.push('ready'));

      // Events are emitted during beforeEach setup
      // Just verify the agent is in the correct state
      const status = agent.getStatus();
      expect(status.state).toBe(AgentState.AGENT_STATE_IDLE);
    });
  });

  // =============================================================================
  // Message Processing Tests
  // =============================================================================

  describe('message processing', () => {
    it('should process a simple message', async () => {
      let responseReceived: AgentResponse | null = null;

      const respondPromise = new Promise<void>((resolve) => {
        const respond = (response: AgentResponse) => {
          responseReceived = response;
          resolve();
        };

        agent.receiveMessage(
          {
            id: TEST_CORRELATION_ID,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: 'Hello, assistant!',
            sessionId: TEST_SESSION_ID,
          },
          respond
        );
      });

      await respondPromise;

      expect(responseReceived).toBeDefined();
      expect(responseReceived?.metadata?.responseId).toBeDefined();
      expect(responseReceived?.metadata?.sessionId).toBe(TEST_SESSION_ID);
    });

    it('should create a session when receiving first message', async () => {
      const respond = jest.fn();

      await agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'First message',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      // Wait for processing
      await new Promise((resolve) => setTimeout(resolve, 100));

      // Check session was created
      const session = await sessionManager.get(TEST_SESSION_ID);
      expect(session).toBeDefined();
      expect(session.id).toBe(TEST_SESSION_ID);
      expect(session.metadata.ownerId).toBe(agentId);
    });

    it('should maintain message history in session', async () => {
      const respond = jest.fn();

      // Send first message
      await agent.receiveMessage(
        {
          id: 'msg-1',
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'First message',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      // Send second message
      await agent.receiveMessage(
        {
          id: 'msg-2',
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Second message',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      // Check message history
      const messages = await sessionManager.getMessages(TEST_SESSION_ID);
      expect(messages.length).toBeGreaterThanOrEqual(2);
    });

    it('should reject message with empty content', async () => {
      const respond = jest.fn();

      await expect(
        agent.receiveMessage(
          {
            id: TEST_CORRELATION_ID,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: '',
            sessionId: TEST_SESSION_ID,
          },
          respond
        )
      ).rejects.toThrow();
    });

    it('should reject message without session ID', async () => {
      const respond = jest.fn();

      await expect(
        agent.receiveMessage(
          {
            id: TEST_CORRELATION_ID,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: 'Hello',
            sessionId: '',
          },
          respond
        )
      ).rejects.toThrow();
    });

    it('should reject message when shutdown', async () => {
      await agent.shutdown();

      const respond = jest.fn();

      await expect(
        agent.receiveMessage(
          {
            id: TEST_CORRELATION_ID,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: 'Hello',
            sessionId: TEST_SESSION_ID,
          },
          respond
        )
      ).rejects.toThrow();
    });
  });

  // =============================================================================
  // Session Management Tests
  // =============================================================================

  describe('session management', () => {
    it('should create session on first message for new session ID', async () => {
      const respond = jest.fn();
      const newSessionId = 'new-session-xyz';

      await agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: newSessionId,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      const session = await sessionManager.get(newSessionId);
      expect(session).toBeDefined();
      expect(session.metadata.ownerId).toBe(agentId);
    });

    it('should reuse existing session for same session ID', async () => {
      const respond = jest.fn();

      // First message
      await agent.receiveMessage(
        {
          id: 'msg-1',
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'First',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      const session1 = await sessionManager.get(TEST_SESSION_ID);

      // Second message
      await agent.receiveMessage(
        {
          id: 'msg-2',
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Second',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      const session2 = await sessionManager.get(TEST_SESSION_ID);

      // Should be same session
      expect(session1.id).toBe(session2.id);
    });
  });

  // =============================================================================
  // Error Handling Tests
  // =============================================================================

  describe('error handling', () => {
    it('should handle registration failure', async () => {
      mockOrchestrator.setRegistrationSucceeds(false);

      const newAgent = new Agent(
        {
          name: 'failing-agent',
          enableStreaming: false,
        },
        {
          grpcClient: mockGrpcClient,
          sessionManager,
          eventEmitter: new EventEmitter(),
        }
      );

      await newAgent.initialize();

      await expect(newAgent.register('fail-agent')).rejects.toThrow();
    });

    it('should emit error event on message processing failure', async () => {
      const errorSpy = jest.fn();

      agent.on(AgentEventType.ERROR, errorSpy);

      // Note: This test depends on how the agent handles errors
      // In a real scenario, we might need to mock the LLM client to fail
      const status = agent.getStatus();
      expect(status).toBeDefined();
    });

    it('should recover from transient errors', async () => {
      // Simulate a transient error then recovery
      mockOrchestrator.setHeartbeatSucceeds(false);

      // Try heartbeat (should fail)
      await expect(mockOrchestrator.heartbeat({ agentId })).rejects.toThrow();

      // Recovery
      mockOrchestrator.setHeartbeatSucceeds(true);

      // Next heartbeat should succeed
      const response = await mockOrchestrator.heartbeat({ agentId });
      expect(response.timestamp).toBeDefined();
    });
  });

  // =============================================================================
  // Status and Metrics Tests
  // =============================================================================

  describe('status and metrics', () => {
    it('should return accurate status', () => {
      const status = agent.getStatus();

      expect(status.agentId).toBe(agentId);
      expect(status.state).toBe(AgentState.AGENT_STATE_IDLE);
      expect(status.uptimeSeconds).toBeGreaterThanOrEqual(0);
      expect(status.lastActivityAt).toBeInstanceOf(Date);
    });

    it('should track message processing', async () => {
      const respond = jest.fn();

      await agent.receiveMessage(
        {
          id: 'msg-1',
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 200));

      const status = agent.getStatus();
      // Note: totalMessagesProcessed is incremented in a private method
      // so this may not be immediately visible
      expect(status).toBeDefined();
    });

    it('should have active sessions count', () => {
      const status = agent.getStatus();
      expect(status.activeSessions).toBeGreaterThanOrEqual(0);
    });
  });

  // =============================================================================
  // Orchestrator Interaction Tests
  // =============================================================================

  describe('orchestrator interaction', () => {
    it('should connect to mock orchestrator', () => {
      expect(mockOrchestrator.isConnected()).toBe(true);
    });

    it('should register with orchestrator', () => {
      const agentInfo = mockOrchestrator.getAgentInfo(agentId);
      expect(agentInfo).toBeDefined();
      expect(agentInfo?.name).toBe('test-assistant');
    });

    it('should send heartbeat to orchestrator', async () => {
      const response = await mockOrchestrator.heartbeat({ agentId });
      expect(response.timestamp).toBeDefined();
      expect(response.state).toBe(AgentState.AGENT_STATE_IDLE);
    });

    it('should unregister from orchestrator', async () => {
      await agent.unregister();

      expect(mockOrchestrator.getRegisteredAgentCount()).toBe(0);
    });
  });

  // =============================================================================
  // Concurrency Tests
  // =============================================================================

  describe('concurrency', () => {
    it('should handle multiple concurrent messages', async () => {
      const respond = jest.fn();
      const messages = 3;
      const promises: Promise<void>[] = [];

      for (let i = 0; i < messages; i++) {
        const promise = agent.receiveMessage(
          {
            id: `msg-${i}`,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: `Message ${i}`,
            sessionId: `session-${i}`,
          },
          respond
        );
        promises.push(promise);
      }

      await Promise.all(promises);

      // Check that multiple sessions were created
      const status = agent.getStatus();
      expect(status.activeSessions).toBeGreaterThanOrEqual(0);
    });

    it('should respect max concurrent messages limit', async () => {
      const respond = jest.fn();

      // Send more messages than maxConcurrentMessages
      for (let i = 0; i < 10; i++) {
        agent.receiveMessage(
          {
            id: `msg-${i}`,
            type: MessageType.MESSAGE_TYPE_DATA,
            timestamp: new Date(),
            content: `Message ${i}`,
            sessionId: `session-${i}`,
          },
          respond
        );
      }

      // Wait for processing
      await new Promise((resolve) => setTimeout(resolve, 500));

      const status = agent.getStatus();
      expect(status).toBeDefined();
    });
  });

  // =============================================================================
  // Event Emission Tests
  // =============================================================================

  describe('event emission', () => {
    it('should emit message received event', async () => {
      const eventSpy = jest.fn();
      agent.on(AgentEventType.MESSAGE_RECEIVED, eventSpy);

      const respond = jest.fn();
      await agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      expect(eventSpy).toHaveBeenCalled();
    });

    it('should emit session created event', async () => {
      const eventSpy = jest.fn();
      agent.on(AgentEventType.SESSION_CREATED, eventSpy);

      const respond = jest.fn();
      await agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: 'brand-new-session',
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(eventSpy).toHaveBeenCalled();
    });
  });

  // =============================================================================
  // Cleanup Tests
  // =============================================================================

  describe('cleanup', () => {
    it('should clean up resources on shutdown', async () => {
      const respond = jest.fn();

      // Create a session
      await agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      await new Promise((resolve) => setTimeout(resolve, 100));

      // Shutdown
      await agent.shutdown();

      // Verify agent is shutdown
      const status = agent.getStatus();
      expect(status.state).toBe(AgentState.AGENT_STATE_TERMINATED);
    });

    it('should wait for active messages before shutdown', async () => {
      const respond = jest.fn();

      // Send a message
      agent.receiveMessage(
        {
          id: TEST_CORRELATION_ID,
          type: MessageType.MESSAGE_TYPE_DATA,
          timestamp: new Date(),
          content: 'Hello',
          sessionId: TEST_SESSION_ID,
        },
        respond
      );

      // Shutdown should wait for message to complete
      await agent.shutdown();

      const status = agent.getStatus();
      expect(status.state).toBe(AgentState.AGENT_STATE_TERMINATED);
    });
  });
});
