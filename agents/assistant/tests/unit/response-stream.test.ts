// response-stream.test.ts - Unit tests for response streaming and aggregation
//
// Copyright 2026 baaaht project

import { describe, it, expect, beforeEach, jest } from '@jest/globals';
import {
  ResponseStreamEventType,
  type ResponseStreamEvent,
  type ResponseStreamEventHandler,
  type ResponseStreamEventFilter,
  type ResponseStreamMiddleware,
  type ResponseStreamOptions,
  type AggregatedResponse,
  ResponseAggregator,
  ResponseStreamHandleImpl,
  createResponseStreamEventHandler,
  createResponseStreamMiddleware,
  responseEventMatchesFilter,
  defaultResponseStreamOptions,
  mergeResponseStreamOptions,
  createResponseStreamId,
  createResponseStreamEvent,
  isContentDelta,
  isToolUseEvent,
  isErrorEvent,
  isCompleteEvent,
} from '../../src/agent/response-stream.js';
import type { ParsedStreamEvent } from '../../src/llm/stream-parser.js';

describe('ResponseStreamEventType', () => {
  it('should have all expected event types', () => {
    expect(ResponseStreamEventType.StreamStart).toBe('stream.start');
    expect(ResponseStreamEventType.StreamChunk).toBe('stream.chunk');
    expect(ResponseStreamEventType.StreamEnd).toBe('stream.end');
    expect(ResponseStreamEventType.StreamError).toBe('stream.error');
    expect(ResponseStreamEventType.TokenDelta).toBe('token.delta');
    expect(ResponseStreamEventType.ContentDelta).toBe('content.delta');
    expect(ResponseStreamEventType.ToolUseStart).toBe('tool_use.start');
    expect(ResponseStreamEventType.ToolUseChunk).toBe('tool_use.chunk');
    expect(ResponseStreamEventType.ToolUseEnd).toBe('tool_use.end');
  });
});

describe('createResponseStreamEventHandler', () => {
  it('should create handler from function', async () => {
    const mockFn = jest.fn();
    const handler = createResponseStreamEventHandler(mockFn);

    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    await handler.handle(event);

    expect(mockFn).toHaveBeenCalledWith(event);
  });

  it('should use default canHandle when not provided', () => {
    const handler = createResponseStreamEventHandler(() => {});

    expect(handler.canHandle(ResponseStreamEventType.ContentDelta)).toBe(true);
    expect(handler.canHandle(ResponseStreamEventType.StreamError)).toBe(true);
  });

  it('should use custom canHandle when provided', () => {
    const canHandleFn = jest.fn((type: ResponseStreamEventType) => type === ResponseStreamEventType.ContentDelta);
    const handler = createResponseStreamEventHandler(() => {}, canHandleFn);

    expect(handler.canHandle(ResponseStreamEventType.ContentDelta)).toBe(true);
    expect(handler.canHandle(ResponseStreamEventType.StreamError)).toBe(false);
    expect(canHandleFn).toHaveBeenCalledTimes(2);
  });
});

describe('createResponseStreamMiddleware', () => {
  it('should create middleware from function', async () => {
    const mockFn = jest.fn().mockResolvedValue({
      id: 'test_1_modified',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    });

    const middleware = createResponseStreamMiddleware(mockFn);

    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    const result = await middleware.process(event);

    expect(mockFn).toHaveBeenCalledWith(event);
    expect(result.id).toBe('test_1_modified');
  });
});

describe('responseEventMatchesFilter', () => {
  const baseEvent: ResponseStreamEvent = {
    id: 'test_1',
    type: ResponseStreamEventType.ContentDelta,
    streamId: 'stream_1',
    provider: 'anthropic',
    model: 'claude-3-5-sonnet-20241022',
    metadata: {
      sessionId: 'session_1',
      correlationId: 'corr_1',
      labels: { key1: 'value1', key2: 'value2' },
    },
  };

  it('should match event with no filter', () => {
    expect(responseEventMatchesFilter(baseEvent, {})).toBe(true);
  });

  it('should match by type', () => {
    expect(
      responseEventMatchesFilter(baseEvent, { type: ResponseStreamEventType.ContentDelta })
    ).toBe(true);
    expect(
      responseEventMatchesFilter(baseEvent, { type: ResponseStreamEventType.StreamError })
    ).toBe(false);
  });

  it('should match by streamId', () => {
    expect(responseEventMatchesFilter(baseEvent, { streamId: 'stream_1' })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { streamId: 'stream_2' })).toBe(false);
  });

  it('should match by provider', () => {
    expect(responseEventMatchesFilter(baseEvent, { provider: 'anthropic' })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { provider: 'openai' })).toBe(false);
  });

  it('should match by model', () => {
    expect(responseEventMatchesFilter(baseEvent, { model: 'claude-3-5-sonnet-20241022' })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { model: 'gpt-4' })).toBe(false);
  });

  it('should match by sessionId', () => {
    expect(responseEventMatchesFilter(baseEvent, { sessionId: 'session_1' })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { sessionId: 'session_2' })).toBe(false);
  });

  it('should match by correlationId', () => {
    expect(responseEventMatchesFilter(baseEvent, { correlationId: 'corr_1' })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { correlationId: 'corr_2' })).toBe(false);
  });

  it('should match by labels (all must match)', () => {
    expect(responseEventMatchesFilter(baseEvent, { labels: { key1: 'value1' } })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { labels: { key1: 'value1', key2: 'value2' } })).toBe(true);
    expect(responseEventMatchesFilter(baseEvent, { labels: { key1: 'wrong' } })).toBe(false);
    expect(responseEventMatchesFilter(baseEvent, { labels: { key3: 'value3' } })).toBe(false);
  });

  it('should match multiple criteria', () => {
    expect(
      responseEventMatchesFilter(baseEvent, {
        type: ResponseStreamEventType.ContentDelta,
        provider: 'anthropic',
        streamId: 'stream_1',
      })
    ).toBe(true);

    expect(
      responseEventMatchesFilter(baseEvent, {
        type: ResponseStreamEventType.StreamError,
        provider: 'anthropic',
      })
    ).toBe(false);
  });

  it('should handle missing metadata', () => {
    const eventNoMetadata: ResponseStreamEvent = {
      id: 'test_2',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_2',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(responseEventMatchesFilter(eventNoMetadata, { sessionId: 'session_1' })).toBe(false);
    expect(responseEventMatchesFilter(eventNoMetadata, { labels: { key1: 'value1' } })).toBe(false);
  });
});

describe('ResponseStreamHandleImpl', () => {
  it('should create handle with correct initial state', () => {
    const handle = new ResponseStreamHandleImpl('stream_1', 'anthropic', 'claude-3-5-sonnet-20241022');

    expect(handle.id).toBe('stream_1');
    expect(handle.provider).toBe('anthropic');
    expect(handle.model).toBe('claude-3-5-sonnet-20241022');
    expect(handle.active).toBe(true);
  });

  it('should close the handle', () => {
    const handle = new ResponseStreamHandleImpl('stream_1', 'anthropic', 'claude-3-5-sonnet-20241022');

    handle.close();

    expect(handle.active).toBe(false);
  });
});

describe('defaultResponseStreamOptions', () => {
  it('should return default options', () => {
    const options = defaultResponseStreamOptions();

    expect(options.bufferSize).toBe(10);
    expect(options.includeUsage).toBe(true);
    expect(options.includeRaw).toBe(false);
    expect(options.callback).toBeUndefined();
    expect(options.middleware).toEqual([]);
  });
});

describe('mergeResponseStreamOptions', () => {
  it('should return defaults when no options provided', () => {
    const options = mergeResponseStreamOptions(undefined);

    expect(options.bufferSize).toBe(10);
    expect(options.includeUsage).toBe(true);
  });

  it('should merge provided options with defaults', () => {
    const options = mergeResponseStreamOptions({
      bufferSize: 20,
      includeUsage: false,
    });

    expect(options.bufferSize).toBe(20);
    expect(options.includeUsage).toBe(false);
    expect(options.includeRaw).toBe(false);
    expect(options.middleware).toEqual([]);
  });

  it('should preserve callback and middleware', () => {
    const callback = jest.fn();
    const middleware = createResponseStreamMiddleware(async (e) => e);

    const options = mergeResponseStreamOptions({
      callback,
      middleware: [middleware],
    });

    expect(options.callback).toBe(callback);
    expect(options.middleware).toEqual([middleware]);
  });
});

describe('createResponseStreamId', () => {
  it('should create unique IDs', () => {
    const id1 = createResponseStreamId();
    const id2 = createResponseStreamId();

    expect(id1).toMatch(/^response_stream_\d+_[a-z0-9]+$/);
    expect(id2).toMatch(/^response_stream_\d+_[a-z0-9]+$/);
    expect(id1).not.toBe(id2);
  });
});

describe('createResponseStreamEvent', () => {
  it('should create event from parsed event', () => {
    const parsedEvent: ParsedStreamEvent = {
      id: 'event_1',
      type: 'content.delta' as any,
      streamId: 'stream_1',
      timestamp: 1234567890,
      data: {
        requestId: 'req_1',
        content: 'Hello, world!',
      },
    };

    const event = createResponseStreamEvent(parsedEvent, 'anthropic', 'claude-3-5-sonnet-20241022');

    expect(event.id).toBe('event_1');
    expect(event.type).toBe('content.delta');
    expect(event.streamId).toBe('stream_1');
    expect(event.provider).toBe('anthropic');
    expect(event.model).toBe('claude-3-5-sonnet-20241022');
    expect(event.data).toEqual({
      requestId: 'req_1',
      content: 'Hello, world!',
    });
    expect(event.metadata?.requestId).toBe('req_1');
    expect(event.metadata?.timestamp).toBe(1234567890);
  });
});

describe('isContentDelta', () => {
  it('should return true for content delta events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      data: { content: 'test' },
    };

    expect(isContentDelta(event)).toBe(true);
  });

  it('should return false for non-content delta events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.StreamError,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isContentDelta(event)).toBe(false);
  });
});

describe('isToolUseEvent', () => {
  it('should return true for tool use start events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ToolUseStart,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      data: { toolCallId: 'tc_1', name: 'test_tool' },
    };

    expect(isToolUseEvent(event)).toBe(true);
  });

  it('should return true for tool use chunk events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ToolUseChunk,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      data: { toolCallId: 'tc_1', name: 'test_tool' },
    };

    expect(isToolUseEvent(event)).toBe(true);
  });

  it('should return true for tool use end events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ToolUseEnd,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      data: { toolCallId: 'tc_1', name: 'test_tool' },
    };

    expect(isToolUseEvent(event)).toBe(true);
  });

  it('should return false for non-tool use events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isToolUseEvent(event)).toBe(false);
  });
});

describe('isErrorEvent', () => {
  it('should return true for error events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.StreamError,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isErrorEvent(event)).toBe(true);
  });

  it('should return false for non-error events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isErrorEvent(event)).toBe(false);
  });
});

describe('isCompleteEvent', () => {
  it('should return true for complete events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.StreamEnd,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isCompleteEvent(event)).toBe(true);
  });

  it('should return false for non-complete events', () => {
    const event: ResponseStreamEvent = {
      id: 'test_1',
      type: ResponseStreamEventType.ContentDelta,
      streamId: 'stream_1',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
    };

    expect(isCompleteEvent(event)).toBe(false);
  });
});

describe('ResponseAggregator', () => {
  let aggregator: ResponseAggregator;

  beforeEach(() => {
    aggregator = new ResponseAggregator();
  });

  describe('process', () => {
    it('should aggregate content from delta events', () => {
      const event1: ParsedStreamEvent = {
        id: 'e1',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'Hello' },
      };

      const event2: ParsedStreamEvent = {
        id: 'e2',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: ', world!' },
      };

      const result1 = aggregator.process(event1);
      expect(result1).toBeNull();
      expect(aggregator.getContent()).toBe('Hello');

      const result2 = aggregator.process(event2);
      expect(result2).toBeNull();
      expect(aggregator.getContent()).toBe('Hello, world!');
    });

    it('should aggregate tool calls from chunk events', () => {
      const event1: ParsedStreamEvent = {
        id: 'e1',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', name: 'test_tool', argumentsDelta: '{"arg1":' },
      };

      const event2: ParsedStreamEvent = {
        id: 'e2',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', argumentsDelta: ' "value1"}' },
      };

      aggregator.process(event1);
      aggregator.process(event2);

      const toolCalls = aggregator.getToolCalls();
      expect(toolCalls).toHaveLength(1);
      expect(toolCalls[0]).toEqual({
        id: 'tc_1',
        name: 'test_tool',
        arguments: '{"arg1": "value1"}',
      });
    });

    it('should handle multiple tool calls', () => {
      aggregator.process({
        id: 'e1',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', name: 'tool1', argumentsDelta: '{}' },
      });

      aggregator.process({
        id: 'e2',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_2', name: 'tool2', argumentsDelta: '{}' },
      });

      const toolCalls = aggregator.getToolCalls();
      expect(toolCalls).toHaveLength(2);
      expect(toolCalls[0].id).toBe('tc_1');
      expect(toolCalls[1].id).toBe('tc_2');
    });

    it('should track usage from events', () => {
      const event: ParsedStreamEvent = {
        id: 'e1',
        type: 'stream.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: {
          usage: {
            inputTokens: 100n,
            outputTokens: 50n,
            totalTokens: 150n,
          },
        },
      };

      aggregator.process(event);

      const result = aggregator.process({
        id: 'e2',
        type: 'stream.end',
        streamId: 's1',
        timestamp: Date.now(),
        data: { response: undefined },
      }) as AggregatedResponse;

      expect(result.usage).toEqual({
        inputTokens: 100n,
        outputTokens: 50n,
        totalTokens: 150n,
      });
    });

    it('should complete on stream end event', () => {
      const completeEvent: ParsedStreamEvent = {
        id: 'e1',
        type: 'stream.end',
        streamId: 's1',
        timestamp: Date.now(),
        data: {
          response: {
            content: 'Final content',
            toolCalls: [],
            usage: { totalTokens: 100n },
          },
        },
      };

      const result = aggregator.process(completeEvent) as AggregatedResponse;

      expect(result).not.toBeNull();
      expect(result.complete).toBe(true);
      expect(result.content).toBe('Final content');
    });

    it('should handle error events', () => {
      const errorEvent: ParsedStreamEvent = {
        id: 'e1',
        type: 'stream.error',
        streamId: 's1',
        timestamp: Date.now(),
        data: {
          code: 'RATE_LIMITED',
          message: 'Rate limit exceeded',
          retryable: true,
        },
      };

      const result = aggregator.process(errorEvent) as AggregatedResponse;

      expect(result.complete).toBe(true);
      expect(result.error).toBeInstanceOf(Error);
      expect(result.error?.message).toBe('Rate limit exceeded');
    });

    it('should enforce max content length', () => {
      const limitedAggregator = new ResponseAggregator({ maxContentLength: 10 });

      limitedAggregator.process({
        id: 'e1',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: '0123456789' },
      });

      const result = limitedAggregator.process({
        id: 'e2',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'X' },
      }) as AggregatedResponse;

      expect(result.complete).toBe(false);
      expect(result.error).toBeDefined();
      expect(result.error?.message).toContain('Maximum content length exceeded');
    });

    it('should enforce max tool calls limit', () => {
      const limitedAggregator = new ResponseAggregator({ maxToolCalls: 2 });

      limitedAggregator.process({
        id: 'e1',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', name: 'tool1', argumentsDelta: '{}' },
      });

      limitedAggregator.process({
        id: 'e2',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_2', name: 'tool2', argumentsDelta: '{}' },
      });

      const result = limitedAggregator.process({
        id: 'e3',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_3', name: 'tool3', argumentsDelta: '{}' },
      }) as AggregatedResponse;

      expect(result.complete).toBe(false);
      expect(result.error).toBeDefined();
      expect(result.error?.message).toContain('Maximum tool calls limit exceeded');
    });

    it('should return final response when complete', () => {
      aggregator.process({
        id: 'e1',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'Hello' },
      });

      const completeEvent: ParsedStreamEvent = {
        id: 'e2',
        type: 'stream.end',
        streamId: 's1',
        timestamp: Date.now(),
        data: {
          response: {
            content: 'Final',
            toolCalls: [{ id: 'tc_1', name: 'tool', arguments: '{}' }],
            usage: { inputTokens: 10n, outputTokens: 20n },
          },
        },
      };

      const result1 = aggregator.process(completeEvent) as AggregatedResponse;
      const result2 = aggregator.process({
        id: 'e3',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'Ignored' },
      }) as AggregatedResponse;

      expect(result1.complete).toBe(true);
      expect(result1.content).toBe('Final');
      expect(result2.complete).toBe(true);
    });
  });

  describe('reset', () => {
    it('should reset aggregator state', () => {
      aggregator.process({
        id: 'e1',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'Hello' },
      });

      aggregator.process({
        id: 'e2',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', name: 'tool', argumentsDelta: '{}' },
      });

      aggregator.reset();

      expect(aggregator.getContent()).toBe('');
      expect(aggregator.getToolCalls()).toEqual([]);
      expect(aggregator.isComplete()).toBe(false);
      expect(aggregator.hasError()).toBe(false);
      expect(aggregator.getError()).toBeUndefined();
    });
  });

  describe('getContent', () => {
    it('should return current content', () => {
      aggregator.process({
        id: 'e1',
        type: 'content.delta',
        streamId: 's1',
        timestamp: Date.now(),
        data: { content: 'Hello' },
      });

      expect(aggregator.getContent()).toBe('Hello');
    });
  });

  describe('getToolCalls', () => {
    it('should return current tool calls', () => {
      aggregator.process({
        id: 'e1',
        type: 'tool_use.chunk',
        streamId: 's1',
        timestamp: Date.now(),
        data: { toolCallId: 'tc_1', name: 'tool', argumentsDelta: '{}' },
      });

      const toolCalls = aggregator.getToolCalls();
      expect(toolCalls).toHaveLength(1);
      expect(toolCalls[0].id).toBe('tc_1');
    });
  });

  describe('isComplete', () => {
    it('should return false initially', () => {
      expect(aggregator.isComplete()).toBe(false);
    });

    it('should return true after complete event', () => {
      aggregator.process({
        id: 'e1',
        type: 'stream.end',
        streamId: 's1',
        timestamp: Date.now(),
        data: { response: undefined },
      });

      expect(aggregator.isComplete()).toBe(true);
    });

    it('should return true after error', () => {
      aggregator.process({
        id: 'e1',
        type: 'stream.error',
        streamId: 's1',
        timestamp: Date.now(),
        data: { code: 'ERROR', message: 'Test error' },
      });

      expect(aggregator.isComplete()).toBe(true);
    });
  });

  describe('hasError', () => {
    it('should return false initially', () => {
      expect(aggregator.hasError()).toBe(false);
    });

    it('should return true after error event', () => {
      aggregator.process({
        id: 'e1',
        type: 'stream.error',
        streamId: 's1',
        timestamp: Date.now(),
        data: { code: 'ERROR', message: 'Test error' },
      });

      expect(aggregator.hasError()).toBe(true);
    });
  });

  describe('getError', () => {
    it('should return undefined when no error', () => {
      expect(aggregator.getError()).toBeUndefined();
    });

    it('should return error after error event', () => {
      aggregator.process({
        id: 'e1',
        type: 'stream.error',
        streamId: 's1',
        timestamp: Date.now(),
        data: { code: 'ERROR', message: 'Test error' },
      });

      const error = aggregator.getError();
      expect(error).toBeInstanceOf(Error);
      expect(error?.message).toBe('Test error');
    });
  });
});
