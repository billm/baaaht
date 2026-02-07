// Package ipc implements inter-process communication (IPC) broker for
// inter-container communication in the baaaht orchestrator.
//
// The IPC system provides:
//
//   - Message routing between containers based on source/target IDs
//   - Session-scoped message history tracking
//   - Authentication and authorization support
//   - Unix domain socket-based communication
//   - Message handler registration for custom processing
//
// The Broker is the main component that manages all IPC operations.
// It uses a Unix domain socket (Socket) for actual data transfer.
//
// Example usage:
//
//	broker, err := ipc.New(cfg, logger)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start the broker
//	if err := broker.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register a handler for messages
//	broker.RegisterHandler("request", myHandler)
//
//	// Send a message
//	msg := &types.IPCMessage{
//	    Source:  sourceContainerID,
//	    Target:  targetContainerID,
//	    Type:    "request",
//	    Payload: data,
//	}
//	if err := broker.Send(ctx, msg); err != nil {
//	    log.Fatal(err)
//	}
package ipc
