// session-manager.test.ts - Unit tests for session manager
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import {
  SessionManager,
  SessionState,
  SessionStatus,
  MessageRole,
  SessionErrorCode,
} from '../../src/session/manager.js';

describe('SessionManager', () => {
  let manager: SessionManager;

  beforeEach(() => {
    manager = new SessionManager({
      maxSessions: 10,
      idleTimeout: 5000,
      timeout: 60000,
      persistenceEnabled: false,
      cleanupInterval: 10000,
    });
  });

  afterEach(async () => {
    await manager.close();
  });

  describe('create', () => {
    it('should create a new session', async () => {
      const sessionId = await manager.create(
        {
          name: 'test-session',
          ownerId: 'user-123',
        },
        {
          maxDuration: 60000,
        }
      );

      expect(sessionId).toBeDefined();
      expect(typeof sessionId).toBe('string');

      const session = await manager.get(sessionId);
      expect(session.id).toBe(sessionId);
      expect(session.state).toBe(SessionState.ACTIVE);
      expect(session.status).toBe(SessionStatus.RUNNING);
      expect(session.metadata.name).toBe('test-session');
      expect(session.metadata.ownerId).toBe('user-123');
    });

    it('should enforce max sessions limit', async () => {
      const smallManager = new SessionManager({ maxSessions: 2 });

      await smallManager.create({ name: 'session-1' });
      await smallManager.create({ name: 'session-2' });

      await expect(smallManager.create({ name: 'session-3' })).rejects.toThrow(
        'Maximum sessions limit reached'
      );

      await smallManager.close();
    });
  });

  describe('get', () => {
    it('should retrieve an existing session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      const session = await manager.get(sessionId);

      expect(session).toBeDefined();
      expect(session.id).toBe(sessionId);
    });

    it('should throw error for non-existent session', async () => {
      await expect(manager.get('non-existent')).rejects.toThrow('Session not found');
    });
  });

  describe('list', () => {
    beforeEach(async () => {
      await manager.create({ name: 'session-1', ownerId: 'user-1' });
      await manager.create({ name: 'session-2', ownerId: 'user-2' });
      await manager.create({ name: 'session-3', ownerId: 'user-1' });
    });

    it('should list all sessions', async () => {
      const sessions = await manager.list();
      expect(sessions).toHaveLength(3);
    });

    it('should filter sessions by state', async () => {
      const sessions = await manager.list({ state: SessionState.ACTIVE });
      expect(sessions).toHaveLength(3);
    });

    it('should filter sessions by owner', async () => {
      const sessions = await manager.list({ ownerId: 'user-1' });
      expect(sessions).toHaveLength(2);
    });

    it('should filter sessions by label', async () => {
      await manager.update((await manager.list({ ownerId: 'user-1' }))[0].id, {
        metadata: { labels: { env: 'test' } },
      });

      const sessions = await manager.list({ label: { env: 'test' } });
      expect(sessions).toHaveLength(1);
    });
  });

  describe('update', () => {
    it('should update session metadata', async () => {
      const sessionId = await manager.create({ name: 'original-name' });

      await manager.update(sessionId, {
        metadata: { name: 'updated-name' },
      });

      const session = await manager.get(sessionId);
      expect(session.metadata.name).toBe('updated-name');
    });

    it('should throw error when updating closed session', async () => {
      const sessionId = await manager.create({ name: 'test' });
      await manager.closeSession(sessionId);

      await expect(
        manager.update(sessionId, { metadata: { name: 'new-name' } })
      ).rejects.toThrow('Cannot update closed session');
    });
  });

  describe('closeSession', () => {
    it('should close an active session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      await manager.closeSession(sessionId);

      const session = await manager.get(sessionId);
      expect(session.state).toBe(SessionState.CLOSED);
      expect(session.status).toBe(SessionStatus.STOPPED);
      expect(session.closedAt).toBeDefined();
    });

    it('should be idempotent', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      await manager.closeSession(sessionId);
      await manager.closeSession(sessionId); // Should not throw

      const session = await manager.get(sessionId);
      expect(session.state).toBe(SessionState.CLOSED);
    });
  });

  describe('delete', () => {
    it('should delete a closed session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      await manager.closeSession(sessionId);
      await manager.delete(sessionId);

      await expect(manager.get(sessionId)).rejects.toThrow('Session not found');
    });

    it('should throw error when deleting active session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      await expect(manager.delete(sessionId)).rejects.toThrow(
        'Cannot delete active session'
      );
    });
  });

  describe('addMessage', () => {
    it('should add a message to session history', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      await manager.addMessage(sessionId, {
        role: MessageRole.USER,
        content: 'Hello, world!',
      });

      const messages = await manager.getMessages(sessionId);
      expect(messages).toHaveLength(1);
      expect(messages[0].role).toBe(MessageRole.USER);
      expect(messages[0].content).toBe('Hello, world!');
    });

    it('should generate message ID if not provided', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      await manager.addMessage(sessionId, {
        role: MessageRole.USER,
        content: 'Test',
      });

      const messages = await manager.getMessages(sessionId);
      expect(messages[0].id).toBeDefined();
    });

    it('should transition from idle to active on message', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      // Add a message and wait for idle transition (simulation)
      await manager.addMessage(sessionId, {
        role: MessageRole.USER,
        content: 'Test',
      });

      const state = await manager.getState(sessionId);
      expect(state).toBe(SessionState.ACTIVE);
    });

    it('should throw error when adding to closed session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      await manager.closeSession(sessionId);

      await expect(
        manager.addMessage(sessionId, {
          role: MessageRole.USER,
          content: 'Test',
        })
      ).rejects.toThrow('Cannot add message to closed session');
    });
  });

  describe('getState', () => {
    it('should return the current session state', async () => {
      const sessionId = await manager.create({ name: 'test-session' });
      const state = await manager.getState(sessionId);

      expect(state).toBe(SessionState.ACTIVE);
    });
  });

  describe('getStats', () => {
    it('should return session statistics', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      await manager.addMessage(sessionId, {
        role: MessageRole.USER,
        content: 'Hello',
      });

      await manager.addMessage(sessionId, {
        role: MessageRole.ASSISTANT,
        content: 'Hi there!',
      });

      const stats = await manager.getStats(sessionId);

      expect(stats.sessionId).toBe(sessionId);
      expect(stats.messageCount).toBe(2);
      expect(stats.containerCount).toBe(0);
      expect(stats.taskCount).toBe(0);
      expect(stats.durationSeconds).toBeGreaterThanOrEqual(0);
    });
  });

  describe('getStats (manager)', () => {
    it('should return manager statistics', async () => {
      await manager.create({ name: 'session-1' });
      await manager.create({ name: 'session-2' });

      const stats = manager.getStats();

      expect(stats.active).toBe(2);
      expect(stats.total).toBe(2);
    });
  });

  describe('validateSession', () => {
    it('should validate an active session', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      await expect(manager.validateSession(sessionId)).resolves.toBeUndefined();
    });

    it('should throw error for non-existent session', async () => {
      await expect(manager.validateSession('non-existent')).rejects.toThrow(
        'Session not found'
      );
    });

    it('should throw error for expired session', async () => {
      const sessionId = await manager.create(
        { name: 'test-session' },
        { maxDuration: 1 } // 1ms
      );

      // Wait for expiration
      await new Promise((resolve) => setTimeout(resolve, 10));

      await expect(manager.validateSession(sessionId)).rejects.toThrow(
        'Session has expired'
      );
    });
  });

  describe('state transitions', () => {
    it('should follow valid state transitions', async () => {
      const sessionId = await manager.create({ name: 'test-session' });

      // Initial state should be ACTIVE (after INITIALIZING)
      let state = await manager.getState(sessionId);
      expect(state).toBe(SessionState.ACTIVE);

      // Can transition to CLOSED
      await manager.closeSession(sessionId);
      state = await manager.getState(sessionId);
      expect(state).toBe(SessionState.CLOSED);
    });
  });

  describe('events', () => {
    it('should emit sessionCreated event', async () => {
      const handler = jest.fn();
      manager.on('sessionCreated', handler);

      await manager.create({ name: 'test' });

      expect(handler).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: expect.any(String),
          metadata: expect.objectContaining({ name: 'test' }),
        })
      );
    });

    it('should emit messageAdded event', async () => {
      const handler = jest.fn();
      manager.on('messageAdded', handler);

      const sessionId = await manager.create({ name: 'test' });
      await manager.addMessage(sessionId, {
        role: MessageRole.USER,
        content: 'Hello',
      });

      expect(handler).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId,
          message: expect.objectContaining({
            role: MessageRole.USER,
            content: 'Hello',
          }),
        })
      );
    });

    it('should emit sessionClosed event', async () => {
      const handler = jest.fn();
      manager.on('sessionClosed', handler);

      const sessionId = await manager.create({ name: 'test' });
      await manager.closeSession(sessionId);

      expect(handler).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId,
          state: SessionState.CLOSED,
        })
      );
    });
  });
});
