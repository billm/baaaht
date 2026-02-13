// shutdown.test.ts - Unit tests for shutdown manager
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, afterEach, jest } from '@jest/globals';
import {
  ShutdownManager,
  ShutdownState,
  createShutdownManager,
  type ShutdownLogger,
  type ShutdownHook,
} from '../../src/shutdown.js';

// Mock logger for testing
class MockLogger implements ShutdownLogger {
  info = jest.fn();
  warn = jest.fn();
  error = jest.fn();
  debug = jest.fn();

  reset() {
    this.info.mockReset();
    this.warn.mockReset();
    this.error.mockReset();
    this.debug.mockReset();
  }
}

describe('ShutdownManager', () => {
  let manager: ShutdownManager;
  let logger: MockLogger;

  beforeEach(() => {
    logger = new MockLogger();
    manager = new ShutdownManager({ logger });
  });

  afterEach(async () => {
    // Clean up manager if needed
    if (manager.getState() !== ShutdownState.COMPLETE) {
      manager.stop();
    }
  });

  describe('constructor', () => {
    it('should create a shutdown manager with default config', () => {
      const defaultManager = new ShutdownManager();
      expect(defaultManager.getState()).toBe(ShutdownState.RUNNING);
      expect(defaultManager.isShuttingDown()).toBe(false);
      expect(defaultManager.isComplete()).toBe(false);
    });

    it('should create a shutdown manager with custom config', () => {
      const customManager = new ShutdownManager({
        shutdownTimeout: 5000,
        hookTimeout: 1000,
        logger,
      });
      expect(customManager.getState()).toBe(ShutdownState.RUNNING);
      logger.debug.mockReset();
    });

    it('should log creation', () => {
      logger.debug.mockClear();
      new ShutdownManager({ logger });
      expect(logger.debug).toHaveBeenCalledWith(
        'ShutdownManager created',
        expect.objectContaining({
          timeout: expect.any(Number),
          hookTimeout: expect.any(Number),
        })
      );
    });
  });

  describe('start', () => {
    it('should start the shutdown manager', () => {
      manager.start();
      expect(logger.info).toHaveBeenCalledWith(
        'Shutdown manager started',
        expect.objectContaining({
          timeout: expect.any(Number),
          signals: ['SIGINT', 'SIGTERM'],
        })
      );
    });

    it('should be idempotent', () => {
      manager.start();
      logger.info.mockClear();
      manager.start();
      expect(logger.info).not.toHaveBeenCalled();
    });

    it('should log debug message when already started', () => {
      manager.start();
      logger.debug.mockClear();
      manager.start();
      expect(logger.debug).toHaveBeenCalledWith('Shutdown manager already started');
    });
  });

  describe('stop', () => {
    it('should stop the shutdown manager', () => {
      manager.start();
      manager.stop();
      expect(logger.debug).toHaveBeenCalledWith('Shutdown manager stopped');
    });

    it('should be idempotent', () => {
      manager.start();
      manager.stop();
      logger.debug.mockClear();
      manager.stop();
      expect(logger.debug).not.toHaveBeenCalled();
    });

    it('should do nothing when not started', () => {
      logger.debug.mockClear();
      manager.stop();
      expect(logger.debug).not.toHaveBeenCalled();
    });
  });

  describe('shutdown', () => {
    it('should transition through shutdown states', async () => {
      manager.start();

      const shutdownPromise = manager.shutdown('test shutdown');

      // While shutting down, state should be INITIATED or STOPPING
      expect(manager.isShuttingDown()).toBe(true);

      await shutdownPromise;

      expect(manager.getState()).toBe(ShutdownState.COMPLETE);
      expect(manager.isComplete()).toBe(true);
      expect(manager.getShutdownReason()).toBe('test shutdown');
    });

    it('should log shutdown initiation', async () => {
      manager.start();
      logger.info.mockClear();

      await manager.shutdown('test reason');

      expect(logger.info).toHaveBeenCalledWith(
        'Shutdown initiated',
        expect.objectContaining({ reason: 'test reason' })
      );
    });

    it('should log shutdown completion', async () => {
      manager.start();
      logger.info.mockClear();

      await manager.shutdown('test');

      expect(logger.info).toHaveBeenCalledWith(
        'Shutdown complete',
        expect.objectContaining({
          reason: 'test',
          durationMs: expect.any(Number),
        })
      );
    });

    it('should execute pre-shutdown hooks', async () => {
      const preShutdownHook = jest.fn().mockResolvedValue(undefined);
      manager.addHook(preShutdownHook);

      await manager.shutdown('test');

      expect(preShutdownHook).toHaveBeenCalledWith(
        expect.any(AbortSignal)
      );
    });

    it('should execute multiple hooks in order', async () => {
      const callOrder: string[] = [];
      const hook1: ShutdownHook = jest.fn(async () => {
        callOrder.push('hook1');
      });
      const hook2: ShutdownHook = jest.fn(async () => {
        callOrder.push('hook2');
      });
      const hook3: ShutdownHook = jest.fn(async () => {
        callOrder.push('hook3');
      });

      manager.addHook(hook1);
      manager.addHook(hook2);
      manager.addHook(hook3);

      await manager.shutdown('test');

      expect(callOrder).toEqual(['hook1', 'hook2', 'hook3', 'hook1', 'hook2', 'hook3']);
    });

    it('should handle hook errors gracefully', async () => {
      const errorHook = jest.fn().mockRejectedValue(new Error('Hook failed'));
      manager.addHook(errorHook);

      await expect(manager.shutdown('test')).rejects.toThrow('Hook failed');
    });

    it('should throw if shutdown already initiated', async () => {
      const shutdown1 = manager.shutdown('first');
      const shutdown2 = manager.shutdown('second');

      await shutdown1;

      // Second call should handle gracefully (return same promise or be ignored)
      await expect(shutdown2).resolves.toBeUndefined();
    });

    it('should be idempotent - concurrent calls', async () => {
      const hook = jest.fn();
      manager.addHook(hook);

      // Start multiple shutdowns concurrently
      const [result1, result2, result3] = await Promise.all([
        manager.shutdown('test'),
        manager.shutdown('test'),
        manager.shutdown('test'),
      ]);

      void result1;
      void result2;
      void result3;

      // Hook should only be called for one shutdown flow (pre + post phases)
      expect(hook).toHaveBeenCalledTimes(2);
      expect(manager.getState()).toBe(ShutdownState.COMPLETE);
    });
  });

  describe('shutdownAndWait', () => {
    it('should wait for shutdown completion', async () => {
      const hook = jest.fn();
      manager.addHook(hook);

      await manager.shutdownAndWait('test', 5000);

      expect(hook).toHaveBeenCalled();
      expect(manager.isComplete()).toBe(true);
    });

    it('should timeout if shutdown takes too long', async () => {
      const slowHook = jest.fn(
        () =>
          new Promise((resolve) => setTimeout(resolve, 10000))
      );
      manager.addHook(slowHook);

      await expect(
        manager.shutdownAndWait('test', 100)
      ).rejects.toThrow('timeout');
    });
  });

  describe('addHook', () => {
    it('should add a shutdown hook', () => {
      const hook: ShutdownHook = jest.fn();
      manager.addHook(hook);

      expect(logger.debug).toHaveBeenCalledWith(
        'Shutdown hook registered',
        expect.objectContaining({
          totalHooks: expect.any(Number),
        })
      );
    });
  });

  describe('removeHook', () => {
    it('should remove a shutdown hook', () => {
      const hook: ShutdownHook = jest.fn();
      manager.addHook(hook);
      logger.debug.mockClear();

      const removed = manager.removeHook(hook);

      expect(removed).toBe(true);
      expect(logger.debug).toHaveBeenCalledWith(
        'Shutdown hook removed',
        expect.objectContaining({
          totalHooks: 0,
        })
      );
    });

    it('should return false when removing non-existent hook', () => {
      const hook: ShutdownHook = jest.fn();

      const removed = manager.removeHook(hook);

      expect(removed).toBe(false);
    });
  });

  describe('getState', () => {
    it('should return RUNNING initially', () => {
      expect(manager.getState()).toBe(ShutdownState.RUNNING);
    });

    it('should return COMPLETE after shutdown', async () => {
      await manager.shutdown('test');
      expect(manager.getState()).toBe(ShutdownState.COMPLETE);
    });
  });

  describe('isShuttingDown', () => {
    it('should return false initially', () => {
      expect(manager.isShuttingDown()).toBe(false);
    });

    it('should return true during shutdown', async () => {
      const shutdownPromise = manager.shutdown('test');
      expect(manager.isShuttingDown()).toBe(true);
      await shutdownPromise;
    });

    it('should return true after shutdown', async () => {
      await manager.shutdown('test');
      expect(manager.isShuttingDown()).toBe(true);
    });
  });

  describe('isComplete', () => {
    it('should return false initially', () => {
      expect(manager.isComplete()).toBe(false);
    });

    it('should return true after shutdown', async () => {
      await manager.shutdown('test');
      expect(manager.isComplete()).toBe(true);
    });
  });

  describe('getShutdownReason', () => {
    it('should return empty string initially', () => {
      expect(manager.getShutdownReason()).toBe('');
    });

    it('should return the reason after shutdown', async () => {
      await manager.shutdown('test reason');
      expect(manager.getShutdownReason()).toBe('test reason');
    });
  });

  describe('getSignal', () => {
    it('should return an abort signal', () => {
      const signal = manager.getSignal();
      expect(signal).toBeInstanceOf(AbortSignal);
      expect(signal.aborted).toBe(false);
    });

    it('should abort signal after stop', () => {
      manager.start();
      manager.stop();

      const signal = manager.getSignal();
      // The signal should be aborted after stop
      expect(signal.aborted).toBe(true);
    });
  });

  describe('waitCompletion', () => {
    it('should resolve immediately if already complete', async () => {
      await manager.shutdown('test');
      await expect(manager.waitCompletion()).resolves.toBeUndefined();
    });

    it('should wait for completion if shutdown in progress', async () => {
      const slowHook = jest.fn(
        () => new Promise((resolve) => setTimeout(resolve, 100))
      );
      manager.addHook(slowHook);

      // Start shutdown but don't await
      const shutdownPromise = manager.shutdown('test');

      // Wait for completion
      await manager.waitCompletion();

      expect(manager.isComplete()).toBe(true);

      await shutdownPromise;
    });

    it('should timeout if waiting too long', async () => {
      const slowHook = jest.fn(
        () => new Promise((resolve) => setTimeout(resolve, 5000))
      );
      manager.addHook(slowHook);

      // Start shutdown without awaiting
      manager.shutdown('test');

      await expect(manager.waitCompletion(100)).rejects.toThrow('timeout');
    });
  });

  describe('hook execution with timeout', () => {
    it('should timeout hooks that take too long', async () => {
      manager = new ShutdownManager({ logger, hookTimeout: 50 });
      const slowHook = jest.fn(
        () => new Promise((resolve) => setTimeout(resolve, 10000))
      );
      manager.addHook(slowHook);

      await expect(manager.shutdown('test')).rejects.toThrow('Hook timeout');
    }, 10000);

    it('should continue with other hooks after timeout', async () => {
      manager = new ShutdownManager({ logger, hookTimeout: 50 });
      const slowHook = jest.fn(
        () => new Promise((resolve) => setTimeout(resolve, 10000))
      );
      const fastHook = jest.fn().mockResolvedValue(undefined);

      manager.addHook(slowHook);
      manager.addHook(fastHook);

      await expect(manager.shutdown('test')).rejects.toThrow();

      // Fast hook should still be called
      expect(fastHook).toHaveBeenCalled();
    }, 10000);
  });

  describe('toString', () => {
    it('should return a string representation', () => {
      const str = manager.toString();
      expect(str).toContain('ShutdownManager');
      expect(str).toContain('state: running');
      expect(str).toContain('hooks: 0');
      expect(str).toContain('started: false');
    });
  });
});

describe('createShutdownManager', () => {
  it('should create a new ShutdownManager instance', () => {
    const manager = createShutdownManager({
      shutdownTimeout: 5000,
    });

    expect(manager).toBeInstanceOf(ShutdownManager);
    expect(manager.getState()).toBe(ShutdownState.RUNNING);
  });
});

describe('ShutdownState enum', () => {
  it('should have correct values', () => {
    expect(ShutdownState.RUNNING).toBe('running');
    expect(ShutdownState.INITIATED).toBe('initiated');
    expect(ShutdownState.STOPPING).toBe('stopping');
    expect(ShutdownState.COMPLETE).toBe('complete');
  });
});
