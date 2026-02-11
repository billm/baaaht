// index.ts - Entry point for the Assistant Agent
//
// This is the main entry point that bootstraps and starts the Assistant Agent.
//
// Copyright 2026 baaaht project

import { bootstrapWithDefaults, closeGlobal } from './bootstrap.js';

/**
 * Main entry point
 */
async function main() {
  console.log('Starting Assistant Agent...');

  // Check if AgentState is available
  try {
    const { AgentState } = await import('./proto/agent.js');
    console.log('AgentState available:', Object.keys(AgentState));
  } catch (e) {
    console.error('AgentState import failed:', e);
  }

  const result = await bootstrapWithDefaults();

  if (result.error) {
    console.error('Failed to start agent:', result.error.message);
    console.error('Step:', (result.error as any).step);
    console.error('Code:', (result.error as any).code);
    process.exit(1);
  }

  console.log(`Agent started successfully`);
  console.log(`  Agent ID: ${result.agentId}`);
  console.log(`  Version: ${result.version}`);
  console.log(`  Orchestrator: ${process.env.ORCHESTRATOR_URL ?? 'localhost:50051'}`);
  console.log(`  LLM Gateway: ${process.env.LLM_GATEWAY_URL ?? 'http://localhost:8080'}`);
  console.log('');
  console.log('Agent is ready and listening for messages...');
  console.log('Press Ctrl+C to shut down.');
}

// Handle graceful shutdown
const shutdown = async (signal: string) => {
  console.log(`\nReceived ${signal}, shutting down gracefully...`);
  try {
    await closeGlobal();
    console.log('Agent shut down successfully');
    process.exit(0);
  } catch (err) {
    console.error('Error during shutdown:', err);
    process.exit(1);
  }
};

process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT', () => shutdown('SIGINT'));

// Handle uncaught errors
process.on('uncaughtException', (err) => {
  console.error('Uncaught exception:', err);
  shutdown('uncaughtException');
});

process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled rejection at:', promise, 'reason:', reason);
  shutdown('unhandledRejection');
});

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
