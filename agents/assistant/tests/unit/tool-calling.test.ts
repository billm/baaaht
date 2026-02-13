// tool-calling.test.ts - Unit tests for tool calling functionality
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, jest } from '@jest/globals';
import {
  ToolCallAggregator,
  ToolCallBuilder,
  ToolCallExecutor,
  ToolResultFormatter,
  ToolCallValidator,
  createToolCallAggregator,
  createToolCallExecutor,
  createToolCallValidator,
  hasToolCalls,
  extractToolCallIds,
} from '../../src/agent/tool-calling.js';
import type { ToolCall, Tool } from '../../src/proto/llm.js';
import type { ToolDefinition, ToolResult } from '../../src/tools/types.js';

describe('ToolCallAggregator', () => {
  let aggregator: ToolCallAggregator;

  beforeEach(() => {
    aggregator = new ToolCallAggregator();
  });

  describe('addChunk', () => {
    it('should aggregate simple tool call', () => {
      const toolCall = aggregator.addChunk('tc_1', 'delegate', '{"target":"worker"}');

      expect(toolCall).not.toBeNull();
    });

    it('should complete tool call when JSON is valid', () => {
      aggregator.addChunk('tc_1', 'delegate', '{"target":"worker",');
      const toolCall = aggregator.addChunk('tc_1', 'delegate', '"operation":"read_file"}');

      expect(toolCall).not.toBeNull();
      expect(toolCall?.id).toBe('tc_1');
      expect(toolCall?.name).toBe('delegate');
      expect(toolCall?.arguments).toBe('{"target":"worker","operation":"read_file"}');
    });

    it('should complete single-chunk tool call', () => {
      const toolCall = aggregator.addChunk('tc_1', 'delegate', '{"target":"worker"}');

      expect(toolCall).not.toBeNull();
      expect(toolCall?.id).toBe('tc_1');
      expect(toolCall?.name).toBe('delegate');
    });

    it('should track multiple tool calls', () => {
      aggregator.addChunk('tc_1', 'delegate', '{"target":"worker"}');
      aggregator.addChunk('tc_2', 'delegate', '{"target":"worker"}');

      const completed = aggregator.getCompleted();
      expect(completed).toHaveLength(2);
    });

    it('should enforce max tool calls limit', () => {
      const limitedAggregator = new ToolCallAggregator({ maxToolCalls: 2 });

      limitedAggregator.addChunk('tc_1', 'delegate', '{"target":"worker"}');
      limitedAggregator.addChunk('tc_2', 'delegate', '{"target":"worker"}');

      expect(() => {
        limitedAggregator.addChunk('tc_3', 'delegate', '{"target":"worker"}');
      }).toThrow('Maximum tool calls limit exceeded');
    });

    it('should handle empty arguments', () => {
      const toolCall = aggregator.addChunk('tc_1', 'delegate', '{}');

      expect(toolCall).not.toBeNull();
      expect(toolCall?.arguments).toBe('{}');
    });
  });

  describe('getPending', () => {
    it('should return pending tool call IDs', () => {
      aggregator.addChunk('tc_1', 'delegate', '{"target":"worker"');
      aggregator.addChunk('tc_2', 'delegate', '{"target":');

      const pending = aggregator.getPending();
      expect(pending).toHaveLength(2);
      expect(pending).toContain('tc_1');
      expect(pending).toContain('tc_2');
    });

    it('should return empty array when no pending calls', () => {
      aggregator.addChunk('tc_1', 'delegate', '{}');

      const pending = aggregator.getPending();
      expect(pending).toHaveLength(0);
    });
  });

  describe('forceComplete', () => {
    it('should complete pending calls with partial data', () => {
      aggregator.addChunk('tc_1', 'delegate', '{"target":"worker"');
      aggregator.addChunk('tc_2', 'delegate', '{}');

      const all = aggregator.forceComplete();

      expect(all).toHaveLength(2);
      expect(all.map((tc) => tc.arguments)).toEqual(
        expect.arrayContaining(['{"target":"worker"', '{}'])
      );
    });

    it('should reset state after force complete', () => {
      aggregator.addChunk('tc_1', 'delegate', '{"target"');
      aggregator.forceComplete();

      expect(aggregator.getPending()).toHaveLength(0);
      expect(aggregator.getCompleted()).toHaveLength(0);
    });
  });

  describe('reset', () => {
    it('should clear all state', () => {
      aggregator.addChunk('tc_1', 'delegate', '{}');
      aggregator.reset();

      expect(aggregator.getPending()).toHaveLength(0);
      expect(aggregator.getCompleted()).toHaveLength(0);
    });
  });
});

describe('ToolCallBuilder', () => {
  describe('buildTools', () => {
    it('should convert tool definitions to LLM tools', () => {
      const toolDefs: ToolDefinition[] = [
        {
          name: 'test_tool',
          description: 'A test tool',
          inputSchema: {
            type: 'object',
            properties: {
              param1: {
                type: 'string',
                description: 'A parameter',
              },
            },
            required: ['param1'],
          },
        },
      ];

      const tools = ToolCallBuilder.buildTools(toolDefs);

      expect(tools).toHaveLength(1);
      expect(tools[0]).toEqual({
        name: 'test_tool',
        description: 'A test tool',
        inputSchema: toolDefs[0].inputSchema,
      });
    });

    it('should handle multiple tools', () => {
      const toolDefs: ToolDefinition[] = [
        {
          name: 'tool1',
          description: 'Tool 1',
          inputSchema: { type: 'object', properties: {} },
        },
        {
          name: 'tool2',
          description: 'Tool 2',
          inputSchema: { type: 'object', properties: {} },
        },
      ];

      const tools = ToolCallBuilder.buildTools(toolDefs);

      expect(tools).toHaveLength(2);
      expect(tools[0].name).toBe('tool1');
      expect(tools[1].name).toBe('tool2');
    });
  });

  describe('createToolDefinition', () => {
    it('should create a valid tool definition', () => {
      const tool = ToolCallBuilder.createToolDefinition(
        'my_tool',
        'My tool',
        {
          type: 'object',
          properties: {
            input: { type: 'string', description: 'Input' },
          },
          required: ['input'],
        }
      );

      expect(tool.name).toBe('my_tool');
      expect(tool.description).toBe('My tool');
      expect(tool.inputSchema.type).toBe('object');
    });
  });

  describe('validateToolDefinition', () => {
    it('should validate correct tool definition', () => {
      const tool: ToolDefinition = {
        name: 'valid_tool',
        description: 'Valid tool',
        inputSchema: {
          type: 'object',
          properties: { param: { type: 'string', description: 'Param' } },
        },
      };

      expect(ToolCallBuilder.validateToolDefinition(tool)).toBe(true);
    });

    it('should reject missing name', () => {
      const tool = {
        description: 'No name',
        inputSchema: { type: 'object', properties: {} },
      } as unknown as ToolDefinition;

      expect(() => ToolCallBuilder.validateToolDefinition(tool)).toThrow('Tool name is required');
    });

    it('should reject missing description', () => {
      const tool = {
        name: 'tool',
        inputSchema: { type: 'object', properties: {} },
      } as unknown as ToolDefinition;

      expect(() => ToolCallBuilder.validateToolDefinition(tool)).toThrow('Tool description is required');
    });

    it('should reject non-object input schema type', () => {
      const tool = {
        name: 'tool',
        description: 'Tool',
        inputSchema: { type: 'string', properties: {} } as unknown as ToolDefinition['inputSchema'],
      };

      expect(() => ToolCallBuilder.validateToolDefinition(tool)).toThrow('inputSchema type must be "object"');
    });

    it('should reject missing properties', () => {
      const tool = {
        name: 'tool',
        description: 'Tool',
        inputSchema: { type: 'object' } as unknown as ToolDefinition['inputSchema'],
      };

      expect(() => ToolCallBuilder.validateToolDefinition(tool)).toThrow('inputSchema properties is required');
    });
  });
});

describe('ToolCallExecutor', () => {
  let executor: ToolCallExecutor;
  let mockExecuteFn: jest.Mock;

  beforeEach(() => {
    executor = new ToolCallExecutor();
    mockExecuteFn = jest.fn();
  });

  describe('executeToolCall', () => {
    it('should execute tool call successfully', async () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'delegate',
        arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
      };

      mockExecuteFn.mockResolvedValue({
        success: true,
        output: 'File content',
      });

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        toolCall,
        availableTools: [],
      };

      const result = await executor.executeToolCall(context, mockExecuteFn);

      expect(result.toolCallId).toBe('tc_1');
      expect(result.toolName).toBe('delegate');
      expect(result.result.success).toBe(true);
      expect(result.result.output).toBe('File content');
      expect(mockExecuteFn).toHaveBeenCalledWith({
        target: 'worker',
        operation: 'read_file',
        parameters: { path: '/tmp/test.txt' },
      });
    });

    it('should handle tool execution errors', async () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'delegate',
        arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
      };

      mockExecuteFn.mockRejectedValue(new Error('Execution failed'));

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        toolCall,
        availableTools: [],
      };

      const result = await executor.executeToolCall(context, mockExecuteFn);

      expect(result.result.success).toBe(false);
      expect(result.result.error).toBe('Execution failed');
    });

    it('should enforce execution timeout', async () => {
      const timeoutExecutor = new ToolCallExecutor({ maxExecutionTime: 100 });

      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'delegate',
        arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
      };

      mockExecuteFn.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(() => resolve({ success: true }), 200);
          })
      );

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        toolCall,
        availableTools: [],
      };

      const result = await timeoutExecutor.executeToolCall(context, mockExecuteFn);

      expect(result.result.success).toBe(false);
      expect(result.result.error).toContain('timeout');
    }, 10000);
  });

  describe('executeToolCalls', () => {
    it('should execute multiple tool calls', async () => {
      const toolCalls: ToolCall[] = [
        {
          id: 'tc_1',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
        },
        {
          id: 'tc_2',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'list_files', parameters: { path: '/tmp' } }),
        },
      ];

      mockExecuteFn
        .mockResolvedValueOnce({ success: true, output: 'File content' })
        .mockResolvedValueOnce({ success: true, output: 'File list' });

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        availableTools: [],
      };

      const results = await executor.executeToolCalls(toolCalls, context, mockExecuteFn);

      expect(results).toHaveLength(2);
      expect(results[0].result.success).toBe(true);
      expect(results[1].result.success).toBe(true);
    });

    it('should stop on error when continueOnError is false', async () => {
      const errorExecutor = new ToolCallExecutor({ continueOnError: false });

      const toolCalls: ToolCall[] = [
        {
          id: 'tc_1',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
        },
        {
          id: 'tc_2',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'list_files', parameters: { path: '/tmp' } }),
        },
      ];

      mockExecuteFn
        .mockResolvedValueOnce({ success: false, error: 'Failed' })
        .mockResolvedValueOnce({ success: true, output: 'Success' });

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        availableTools: [],
      };

      const results = await errorExecutor.executeToolCalls(toolCalls, context, mockExecuteFn);

      // Should only execute first call
      expect(results).toHaveLength(1);
      expect(results[0].result.success).toBe(false);
      expect(mockExecuteFn).toHaveBeenCalledTimes(1);
    });

    it('should continue on error when continueOnError is true', async () => {
      const continueExecutor = new ToolCallExecutor({ continueOnError: true });

      const toolCalls: ToolCall[] = [
        {
          id: 'tc_1',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'read_file', parameters: { path: '/tmp/test.txt' } }),
        },
        {
          id: 'tc_2',
          name: 'delegate',
          arguments: JSON.stringify({ target: 'worker', operation: 'list_files', parameters: { path: '/tmp' } }),
        },
      ];

      mockExecuteFn
        .mockResolvedValueOnce({ success: false, error: 'Failed' })
        .mockResolvedValueOnce({ success: true, output: 'Success' });

      const context = {
        sessionId: 'sess_1',
        messageId: 'msg_1',
        availableTools: [],
      };

      const results = await continueExecutor.executeToolCalls(toolCalls, context, mockExecuteFn);

      expect(results).toHaveLength(2);
      expect(mockExecuteFn).toHaveBeenCalledTimes(2);
    });
  });

  describe('toToolCallInfo', () => {
    it('should convert execution result to ToolCallInfo', () => {
      const execResult = {
        toolCallId: 'tc_1',
        toolName: 'delegate',
        result: { success: true, output: 'Success' },
        durationMs: 100,
        timestamp: new Date('2026-02-10T00:00:00Z'),
      };

      const info = ToolCallExecutor.toToolCallInfo(execResult);

      expect(info.id).toBe('tc_1');
      expect(info.name).toBe('delegate');
      expect(info.success).toBe(true);
      expect(info.durationMs).toBe(100);
      expect(info.timestamp).toEqual(new Date('2026-02-10T00:00:00Z'));
    });
  });
});

describe('ToolResultFormatter', () => {
  describe('formatResult', () => {
    it('should format successful result', () => {
      const result = {
        toolCallId: 'tc_1',
        toolName: 'delegate',
        result: { success: true, output: 'Operation successful' },
        durationMs: 100,
        timestamp: new Date(),
      };

      const message = ToolResultFormatter.formatResult(result);

      expect(message.role).toBe('user');
      expect(message.content).toBe('Operation successful');
    });

    it('should format successful result with data', () => {
      const result = {
        toolCallId: 'tc_1',
        toolName: 'delegate',
        result: { success: true, data: { key: 'value' } },
        durationMs: 100,
        timestamp: new Date(),
      };

      const message = ToolResultFormatter.formatResult(result);

      expect(message.content).toContain('key');
      expect(message.content).toContain('value');
    });

    it('should format error result', () => {
      const result = {
        toolCallId: 'tc_1',
        toolName: 'delegate',
        result: { success: false, error: 'Tool failed' },
        durationMs: 100,
        timestamp: new Date(),
      };

      const message = ToolResultFormatter.formatResult(result);

      expect(message.role).toBe('user');
      expect(message.content).toContain('Error executing tool');
      expect(message.content).toContain('delegate');
      expect(message.content).toContain('Tool failed');
    });
  });

  describe('formatResults', () => {
    it('should format multiple results', () => {
      const results = [
        {
          toolCallId: 'tc_1',
          toolName: 'delegate',
          result: { success: true, output: 'Success 1' },
          durationMs: 100,
          timestamp: new Date(),
        },
        {
          toolCallId: 'tc_2',
          toolName: 'delegate',
          result: { success: false, error: 'Error' },
          durationMs: 100,
          timestamp: new Date(),
        },
      ];

      const messages = ToolResultFormatter.formatResults(results);

      expect(messages).toHaveLength(2);
      expect(messages[0].role).toBe('user');
      expect(messages[1].role).toBe('user');
    });
  });

  describe('buildToolResultMessage', () => {
    it('should build message from successful results', () => {
      const results = [
        {
          toolCallId: 'tc_1',
          toolName: 'read_file',
          result: { success: true, output: 'file content' },
          durationMs: 100,
          timestamp: new Date(),
        },
      ];

      const message = ToolResultFormatter.buildToolResultMessage(results);

      expect(message).toContain('read_file');
      expect(message).toContain('completed successfully');
      expect(message).toContain('file content');
    });

    it('should build message from failed results', () => {
      const results = [
        {
          toolCallId: 'tc_1',
          toolName: 'read_file',
          result: { success: false, error: 'File not found' },
          durationMs: 100,
          timestamp: new Date(),
        },
      ];

      const message = ToolResultFormatter.buildToolResultMessage(results);

      expect(message).toContain('read_file');
      expect(message).toContain('failed');
      expect(message).toContain('File not found');
    });
  });
});

describe('ToolCallValidator', () => {
  let validator: ToolCallValidator;
  let toolDefs: ToolDefinition[];

  beforeEach(() => {
    toolDefs = [
      {
        name: 'test_tool',
        description: 'A test tool',
        inputSchema: {
          type: 'object',
          properties: {
            string_param: { type: 'string', description: 'String param' },
            number_param: { type: 'number', description: 'Number param' },
            boolean_param: { type: 'boolean', description: 'Boolean param' },
            array_param: { type: 'array', description: 'Array param' },
            object_param: { type: 'object', description: 'Object param' },
            optional_param: { type: 'string', description: 'Optional param' },
          },
          required: ['string_param'],
        },
      },
    ];
    validator = new ToolCallValidator(toolDefs);
  });

  describe('validateToolCall', () => {
    it('should validate correct tool call', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({ string_param: 'value' }),
      };

      expect(validator.validateToolCall(toolCall)).toBe(true);
    });

    it('should reject unknown tool', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'unknown_tool',
        arguments: '{}',
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('Tool not found');
    });

    it('should reject invalid JSON', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: '{invalid json}',
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('Invalid JSON');
    });

    it('should reject missing required parameter', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({ optional_param: 'value' }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('Missing required parameter');
    });

    it('should validate string parameter type', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({ string_param: 123 }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('must be a string');
    });

    it('should validate number parameter type', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({
          string_param: 'value',
          number_param: 'not a number',
        }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('must be a number');
    });

    it('should validate boolean parameter type', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({
          string_param: 'value',
          boolean_param: 'not a boolean',
        }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('must be a boolean');
    });

    it('should validate array parameter type', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({
          string_param: 'value',
          array_param: 'not an array',
        }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('must be an array');
    });

    it('should validate object parameter type', () => {
      const toolCall: ToolCall = {
        id: 'tc_1',
        name: 'test_tool',
        arguments: JSON.stringify({
          string_param: 'value',
          object_param: 'not an object',
        }),
      };

      expect(() => validator.validateToolCall(toolCall)).toThrow('must be an object');
    });
  });

  describe('getAvailableToolNames', () => {
    it('should return available tool names', () => {
      const names = validator.getAvailableToolNames();

      expect(names).toEqual(['test_tool']);
    });
  });
});

describe('Utility Functions', () => {
  describe('createToolCallAggregator', () => {
    it('should create aggregator with config', () => {
      const aggregator = createToolCallAggregator({ maxToolCalls: 5 });

      expect(aggregator).toBeInstanceOf(ToolCallAggregator);
    });

    it('should create aggregator with defaults', () => {
      const aggregator = createToolCallAggregator();

      expect(aggregator).toBeInstanceOf(ToolCallAggregator);
    });
  });

  describe('createToolCallExecutor', () => {
    it('should create executor with config', () => {
      const executor = createToolCallExecutor({ maxExecutionTime: 60000 });

      expect(executor).toBeInstanceOf(ToolCallExecutor);
    });

    it('should create executor with defaults', () => {
      const executor = createToolCallExecutor();

      expect(executor).toBeInstanceOf(ToolCallExecutor);
    });
  });

  describe('createToolCallValidator', () => {
    it('should create validator', () => {
      const tools: ToolDefinition[] = [
        {
          name: 'tool',
          description: 'Tool',
          inputSchema: { type: 'object', properties: {} },
        },
      ];

      const validator = createToolCallValidator(tools);

      expect(validator).toBeInstanceOf(ToolCallValidator);
    });
  });

  describe('hasToolCalls', () => {
    it('should return true for tool calls present', () => {
      const toolCalls: ToolCall[] = [
        { id: 'tc_1', name: 'tool', arguments: '{}' },
      ];

      expect(hasToolCalls(toolCalls)).toBe(true);
    });

    it('should return false for undefined', () => {
      expect(hasToolCalls(undefined)).toBe(false);
    });

    it('should return false for empty array', () => {
      expect(hasToolCalls([])).toBe(false);
    });
  });

  describe('extractToolCallIds', () => {
    it('should extract tool call IDs', () => {
      const toolCalls: ToolCall[] = [
        { id: 'tc_1', name: 'tool', arguments: '{}' },
        { id: 'tc_2', name: 'tool', arguments: '{}' },
        { id: '', name: 'tool', arguments: '{}' },
      ];

      const ids = extractToolCallIds(toolCalls);

      expect(ids).toEqual(['tc_1', 'tc_2']);
    });

    it('should handle undefined IDs', () => {
      const toolCalls: ToolCall[] = [
        { name: 'tool', arguments: '{}' },
        { id: 'tc_1', name: 'tool', arguments: '{}' },
      ];

      const ids = extractToolCallIds(toolCalls);

      expect(ids).toEqual(['tc_1']);
    });
  });
});
