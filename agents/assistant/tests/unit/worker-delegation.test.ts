// worker-delegation.test.ts - Unit tests for Worker agent delegation
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, jest } from '@jest/globals';
import type { Client } from '@grpc/grpc-js';
import {
  WorkerDelegation,
  validateWorkerOperation,
  isWorkerOperation,
  createWorkerDelegation,
} from '../../src/tools/worker-delegation.js';
import {
  DelegateOperation,
  type DelegateParams,
  type ReadFileParams,
  type WriteFileParams,
  type WebSearchParams,
} from '../../src/tools/types.js';
import { TaskState, TaskType } from '../../src/proto/agent.js';

// Mock gRPC client
const createMockGrpcClient = (): Partial<Client> => ({
  executeTask: jest.fn(),
  getTaskStatus: jest.fn(),
});

describe('WorkerDelegation', () => {
  let mockClient: Partial<Client>;
  let delegation: WorkerDelegation;

  beforeEach(() => {
    mockClient = createMockGrpcClient();
    delegation = new WorkerDelegation(mockClient as Client, {
      defaultTimeout: 5000,
    });
  });

  describe('constructor', () => {
    it('should create a new WorkerDelegation instance', () => {
      expect(delegation).toBeInstanceOf(WorkerDelegation);
    });

    it('should use default configuration when not provided', () => {
      const defaultDelegation = new WorkerDelegation(mockClient as Client);
      const config = defaultDelegation.getConfig();

      expect(config.defaultTimeout).toBe(60000);
      expect(config.maxTimeout).toBe(300000);
      expect(config.enableStreaming).toBe(true);
    });

    it('should merge custom configuration with defaults', () => {
      const customDelegation = new WorkerDelegation(mockClient as Client, {
        defaultTimeout: 10000,
        workerAgentId: 'worker-123',
      });
      const config = customDelegation.getConfig();

      expect(config.defaultTimeout).toBe(10000);
      expect(config.workerAgentId).toBe('worker-123');
      expect(config.maxTimeout).toBe(300000); // Default preserved
    });
  });

  describe('delegate', () => {
    it('should delegate read_file operation successfully', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.READ_FILE,
        parameters: {
          path: '/path/to/file.txt',
          offset: 0,
          limit: 1000,
        } as ReadFileParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, { taskId: 'task-123' });
      });

      const mockGetTaskStatus = mockClient.getTaskStatus as jest.MockedFunction<any>;
      mockGetTaskStatus.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, {
          task: {
            taskId: 'task-123',
            state: TaskState.TASK_STATE_COMPLETED,
            result: {
              outputText: 'file content here',
            },
            agentId: 'worker-456',
          },
        });
      });

      const result = await delegation.delegate(params, 'session-123');

      expect(result.success).toBe(true);
      expect(result.taskId).toBe('task-123');
      expect(result.output).toBe('file content here');
      expect(result.taskState).toBe(TaskState.TASK_STATE_COMPLETED);
      expect(result.metadata?.agentId).toBe('worker-456');
      expect(mockExecuteTask).toHaveBeenCalledWith(
        expect.objectContaining({
          type: TaskType.TASK_TYPE_FILE_OPERATION,
          config: expect.objectContaining({
            command: 'read_file',
            arguments: expect.arrayContaining(['/path/to/file.txt']),
          }),
        }),
        expect.any(Object),
        expect.any(Function)
      );
    });

    it('should delegate write_file operation successfully', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.WRITE_FILE,
        parameters: {
          path: '/path/to/file.txt',
          content: 'hello world',
          createParents: true,
        } as WriteFileParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, { taskId: 'task-456' });
      });

      const mockGetTaskStatus = mockClient.getTaskStatus as jest.MockedFunction<any>;
      mockGetTaskStatus.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, {
          task: {
            taskId: 'task-456',
            state: TaskState.TASK_STATE_COMPLETED,
            agentId: 'worker-789',
          },
        });
      });

      const result = await delegation.delegate(params, 'session-456');

      expect(result.success).toBe(true);
      expect(result.taskId).toBe('task-456');
      expect(mockExecuteTask).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({
            command: 'write_file',
            environment: expect.objectContaining({
              FILE_CONTENT: 'hello world',
            }),
          }),
        }),
        expect.any(Object),
        expect.any(Function)
      );
    });

    it('should delegate web_search operation successfully', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.WEB_SEARCH,
        parameters: {
          query: 'typescript testing',
          maxResults: 10,
        } as WebSearchParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, { taskId: 'task-789' });
      });

      const mockGetTaskStatus = mockClient.getTaskStatus as jest.MockedFunction<any>;
      mockGetTaskStatus.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, {
          task: {
            taskId: 'task-789',
            state: TaskState.TASK_STATE_COMPLETED,
            result: {
              outputText: '[{"url": "example.com", "title": "Test"}]',
            },
          },
        });
      });

      const result = await delegation.delegate(params, 'session-789');

      expect(result.success).toBe(true);
      expect(mockExecuteTask).toHaveBeenCalledWith(
        expect.objectContaining({
          type: TaskType.TASK_TYPE_NETWORK_REQUEST,
          config: expect.objectContaining({
            command: 'web_search',
            arguments: expect.arrayContaining(['typescript testing']),
          }),
        }),
        expect.any(Object),
        expect.any(Function)
      );
    });

    it('should handle task failure gracefully', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.READ_FILE,
        parameters: { path: '/nonexistent.txt' } as ReadFileParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, { taskId: 'task-error' });
      });

      const mockGetTaskStatus = mockClient.getTaskStatus as jest.MockedFunction<any>;
      mockGetTaskStatus.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, {
          task: {
            taskId: 'task-error',
            state: TaskState.TASK_STATE_FAILED,
            error: {
              message: 'File not found',
            },
          },
        });
      });

      const result = await delegation.delegate(params, 'session-error');

      expect(result.success).toBe(false);
      expect(result.error).toBe('File not found');
      expect(result.taskState).toBe(TaskState.TASK_STATE_FAILED);
    });

    it('should handle gRPC execution errors', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.READ_FILE,
        parameters: { path: '/test.txt' } as ReadFileParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(new Error('gRPC connection failed'), null);
      });

      const result = await delegation.delegate(params, 'session-grpc-error');

      expect(result.success).toBe(false);
      expect(result.error).toContain('Delegation failed');
      expect(result.error).toContain('gRPC connection failed');
    });

    it('should include metadata in result', async () => {
      const params: DelegateParams = {
        target: 'worker' as any,
        operation: DelegateOperation.READ_FILE,
        parameters: { path: '/test.txt' } as ReadFileParams,
      };

      const mockExecuteTask = mockClient.executeTask as jest.MockedFunction<any>;
      mockExecuteTask.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, { taskId: 'task-metadata' });
      });

      const mockGetTaskStatus = mockClient.getTaskStatus as jest.MockedFunction<any>;
      mockGetTaskStatus.mockImplementation((_req: any, _options: any, callback: any) => {
        callback(null, {
          task: {
            taskId: 'task-metadata',
            state: TaskState.TASK_STATE_COMPLETED,
            agentId: 'worker-meta',
          },
        });
      });

      const result = await delegation.delegate(params, 'session-meta');

      expect(result.metadata).toBeDefined();
      expect(result.metadata?.target).toBe('worker');
      expect(result.metadata?.operation).toBe(DelegateOperation.READ_FILE);
      expect(result.metadata?.createdAt).toBeInstanceOf(Date);
      expect(result.metadata?.completedAt).toBeInstanceOf(Date);
      expect(result.metadata?.duration).toBeGreaterThanOrEqual(0);
      expect(result.metadata?.agentId).toBe('worker-meta');
    });
  });

  describe('updateConfig', () => {
    it('should update configuration values', () => {
      delegation.updateConfig({
        defaultTimeout: 30000,
        workerAgentId: 'new-worker',
      });

      const config = delegation.getConfig();

      expect(config.defaultTimeout).toBe(30000);
      expect(config.workerAgentId).toBe('new-worker');
      expect(config.maxTimeout).toBe(300000); // Unchanged
    });
  });

  describe('getConfig', () => {
    it('should return a copy of the configuration', () => {
      const config1 = delegation.getConfig();
      const config2 = delegation.getConfig();

      expect(config1).toEqual(config2);
      expect(config1).not.toBe(config2); // Different object references
    });
  });
});

describe('createWorkerDelegation', () => {
  it('should create a WorkerDelegation instance', () => {
    const mockClient = createMockGrpcClient();
    const delegation = createWorkerDelegation(mockClient as Client);

    expect(delegation).toBeInstanceOf(WorkerDelegation);
  });

  it('should pass configuration to the instance', () => {
    const mockClient = createMockGrpcClient();
    const delegation = createWorkerDelegation(mockClient as Client, {
      defaultTimeout: 20000,
      workerAgentId: 'worker-factory',
    });

    const config = delegation.getConfig();
    expect(config.defaultTimeout).toBe(20000);
    expect(config.workerAgentId).toBe('worker-factory');
  });
});

describe('validateWorkerOperation', () => {
  describe('read_file validation', () => {
    it('should validate valid read_file parameters', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.READ_FILE, {
          path: '/valid/path.txt',
        });
      }).not.toThrow();
    });

    it('should reject read_file without path', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.READ_FILE, {});
      }).toThrow('read_file requires a valid "path" parameter');
    });

    it('should reject read_file with invalid path type', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.READ_FILE, {
          path: 123 as any,
        });
      }).toThrow('read_file requires a valid "path" parameter');
    });
  });

  describe('write_file validation', () => {
    it('should validate valid write_file parameters', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WRITE_FILE, {
          path: '/valid/path.txt',
          content: 'file content',
        });
      }).not.toThrow();
    });

    it('should reject write_file without path', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WRITE_FILE, {
          content: 'content',
        });
      }).toThrow('write_file requires a valid "path" parameter');
    });

    it('should reject write_file without content', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WRITE_FILE, {
          path: '/path.txt',
        });
      }).toThrow('write_file requires a valid "content" parameter');
    });

    it('should reject write_file with invalid content type', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WRITE_FILE, {
          path: '/path.txt',
          content: undefined as any,
        });
      }).toThrow('write_file requires a valid "content" parameter');
    });
  });

  describe('web_search validation', () => {
    it('should validate valid web_search parameters', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WEB_SEARCH, {
          query: 'search query',
        });
      }).not.toThrow();
    });

    it('should reject web_search without query', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WEB_SEARCH, {});
      }).toThrow('web_search requires a valid "query" parameter');
    });
  });

  describe('web_fetch validation', () => {
    it('should validate valid web_fetch parameters', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WEB_FETCH, {
          url: 'https://example.com',
        });
      }).not.toThrow();
    });

    it('should reject web_fetch without url', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.WEB_FETCH, {});
      }).toThrow('web_fetch requires a valid "url" parameter');
    });
  });

  describe('search_files validation', () => {
    it('should validate valid search_files parameters', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.SEARCH_FILES, {
          path: '/search/path',
          pattern: '*.ts',
        });
      }).not.toThrow();
    });

    it('should reject search_files without path', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.SEARCH_FILES, {
          pattern: '*.ts',
        });
      }).toThrow('search_files requires a valid "path" parameter');
    });

    it('should reject search_files without pattern', () => {
      expect(() => {
        validateWorkerOperation(DelegateOperation.SEARCH_FILES, {
          path: '/search/path',
        });
      }).toThrow('search_files requires a valid "pattern" parameter');
    });
  });
});

describe('isWorkerOperation', () => {
  it('should return true for file operations', () => {
    expect(isWorkerOperation(DelegateOperation.READ_FILE)).toBe(true);
    expect(isWorkerOperation(DelegateOperation.WRITE_FILE)).toBe(true);
    expect(isWorkerOperation(DelegateOperation.DELETE_FILE)).toBe(true);
    expect(isWorkerOperation(DelegateOperation.LIST_FILES)).toBe(true);
    expect(isWorkerOperation(DelegateOperation.SEARCH_FILES)).toBe(true);
  });

  it('should return true for web operations', () => {
    expect(isWorkerOperation(DelegateOperation.WEB_SEARCH)).toBe(true);
    expect(isWorkerOperation(DelegateOperation.WEB_FETCH)).toBe(true);
  });

  it('should return false for non-worker operations', () => {
    expect(isWorkerOperation(DelegateOperation.DEEP_RESEARCH)).toBe(false);
    expect(isWorkerOperation(DelegateOperation.SYNTHESIZE_SOURCES)).toBe(false);
    expect(isWorkerOperation(DelegateOperation.ANALYZE_CODE)).toBe(false);
    expect(isWorkerOperation(DelegateOperation.GENERATE_CODE)).toBe(false);
  });
});
