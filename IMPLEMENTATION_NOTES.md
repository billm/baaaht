# TUI Chat Routing Implementation Notes

## Completed Steps

1. **Orchestrator Changes** (pkg/grpc/orchestrator_service.go):
   - Added `sessionStreams` map to track StreamMessages streams by session ID
   - Modified StreamMessages to register streams on first message
   - Implemented `routeToAssistant` to find and route user messages to assistant agent
   - Implemented `SendResponseToSession` to route assistant responses back to TUI
   - Removed echo logic - user messages now route to assistant instead of echoing

2. **Agent Service Changes** (pkg/grpc/agent_service.go):
   - Added `agentStreams` map to track StreamAgent connections by agent ID
   - Added `messageHandlers` map to store response callbacks
   - Modified StreamAgent to register agent streams on connection
   - Implemented `RouteMessageToAgent` to send messages to agents via StreamAgent
   - Updated StreamAgent to handle incoming agent responses and route back to TUI via messageHandlers

3. **Bootstrap Changes** (pkg/grpc/bootstrap.go):
   - Updated serviceDependencies to include AgentService
   - Reordered service creation to create AgentService first
   - Wired AgentService into OrchestratorService dependencies

## Remaining Work

### Assistant Agent Changes (agents/assistant/src/agent.ts)

The assistant agent needs to:

1. **Import StreamAgentClient**:
   ```typescript
   import { StreamAgentClient, StreamEventType } from './orchestrator/stream-client.js';
   ```

2. **Establish StreamAgent connection after registration** in the `register()` method:
   ```typescript
   // Create and connect StreamAgentClient
   const { StreamAgentClient, StreamEventType } = await import('./orchestrator/stream-client.js');
   this.streamClient = new StreamAgentClient(this.dependencies.grpcClient, this.agentId);
   
   // Set up message handler
   this.streamClient.on(StreamEventType.MESSAGE, async (event: any) => {
     await this.handleOrchestratorMessage(event.data);
   });
   
   // Connect the stream
   await this.streamClient.connect();
   ```

3. **Implement handleOrchestratorMessage** to process incoming messages:
   ```typescript
   private async handleOrchestratorMessage(agentMessage: AgentMessage): Promise<void> {
     // Extract message content from DataMessage payload
     if (!agentMessage.payload || !('dataMessage' in agentMessage.payload)) {
       return;
     }
     
     const dataMsg = agentMessage.payload.dataMessage;
     const content = new TextDecoder().decode(dataMsg.data);
     const sessionId = agentMessage.metadata?.sessionId || '';
     
     // Create respond callback that sends via stream
     const respond = async (response: AgentResponse) => {
       await this.sendResponseViaStream(sessionId, agentMessage.id!, response.content);
     };
     
     // Use existing receiveMessage flow
     await this.receiveMessage(
       {
         id: agentMessage.id,
         content,
         sessionId,
         type: agentMessage.type,
       } as any,
       respond
     );
   }
   ```

4. **Implement sendResponseViaStream** to send responses back:
   ```typescript
   private async sendResponseViaStream(sessionId: string, correlationId: string, content: string): Promise<void> {
     if (!this.streamClient || !this.streamClient.isConnected()) {
       throw new Error('Stream client not connected');
     }
     
     const responseMsg: AgentMessage = {
       id: generateMessageId(),
       type: MessageType.MESSAGE_TYPE_DATA,
       timestamp: new Date(),
       sourceId: this.agentId,
       targetId: 'orchestrator',
       payload: {
         dataMessage: {
           contentType: 'text/plain',
           data: new TextEncoder().encode(content),
         },
       },
       metadata: {
         sessionId,
         correlationId,
       },
     };
     
     await this.streamClient.sendMessage(responseMsg);
   }
   ```

5. **Close stream on shutdown** in the `shutdown()` method:
   ```typescript
   if (this.streamClient) {
     await this.streamClient.close();
     this.streamClient = null;
   }
   ```

### TUI Role Mapping (pkg/tui/model.go)

The TUI should not echo user messages. The role mapping logic is already correct - it defaults to "assistant" for any non-explicitly-mapped role, which means USER messages from the orchestrator would be treated as assistant messages. However, since we removed the echo logic in the orchestrator, this should no longer be an issue.

If needed, add explicit USER role handling:
```go
case proto.MessageRole_MESSAGE_ROLE_USER:
    role = "user"
```

## Testing

1. Start orchestrator with gRPC server
2. Start LLM Gateway
3. Start assistant agent (which will register and establish StreamAgent connection)
4. Start TUI
5. Create session in TUI
6. Send a message
7. Verify:
   - Message routes from TUI → orchestrator → assistant
   - Assistant processes with LLM
   - Response routes back: assistant → orchestrator → TUI
   - TUI displays assistant response (not echo)

## Architecture Flow

```
TUI StreamMessages
    ↓ (user message)
Orchestrator.StreamMessages
    ↓ routeToAssistant()
AgentService.RouteMessageToAgent()
    ↓ (via StreamAgent)
Assistant Agent (receives via StreamAgentClient)
    ↓ processMessage()
    ↓ LLM Gateway
    ↓ sendResponseViaStream()
AgentService.StreamAgent (receives response)
    ↓ messageHandler.SendResponseToSession()
Orchestrator.SendResponseToSession()
    ↓ (via sessionStreams map)
TUI StreamMessages (displays assistant response)
```
