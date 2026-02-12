// grpc-client.ts - gRPC client wrapper for Orchestrator communication
//
// This file provides a TypeScript wrapper around the gRPC client for
// communicating with the Orchestrator's AgentService.
//
// Copyright 2026 baaaht project

import { credentials, Client, ChannelCredentials, loadPackageDefinition } from '@grpc/grpc-js';
import { loadSync } from '@grpc/proto-loader';
import { fileURLToPath } from 'url';
import path from 'path';
import type {
  RegisterRequest,
  RegisterResponse,
  UnregisterRequest,
  UnregisterResponse,
  HeartbeatRequest,
  HeartbeatResponse,
} from '../proto/agent.js';

/**
 * OrchestratorClientConfig holds the configuration for the Orchestrator gRPC client
 */
export interface OrchestratorClientConfig {
  /**
   * The address of the orchestrator gRPC server
   * Defaults to 'localhost:50051' for TCP or a Unix domain socket path
   */
  address?: string;

  /**
   * Whether to use a Unix domain socket instead of TCP
   */
  useUnixSocket?: boolean;

  /**
   * Channel credentials for the gRPC connection
   * Defaults to insecure credentials for local development
   */
  credentials?: ChannelCredentials;

  /**
   * Request timeout in milliseconds
   * Defaults to 30000 (30 seconds)
   */
  timeout?: number;
}

/**
 * OrchestratorClient provides a wrapper around the gRPC client
 * for communicating with the Orchestrator's AgentService.
 *
 * The client handles:
 * - Connection management to the orchestrator
 * - Agent registration
 * - Heartbeat/keepalive
 * - Unregistration
 *
 * Usage:
 * ```typescript
 * const client = new OrchestratorClient({
 *   address: '/tmp/orchestrator.sock',
 *   useUnixSocket: true,
 * });
 *
 * await client.connect();
 * const response = await client.register({
 *   name: 'assistant',
 *   type: AgentType.AGENT_TYPE_WORKER,
 * });
 * ```
 */
export class OrchestratorClient {
  private client: Client | null = null;
  private config: Required<OrchestratorClientConfig>;
  private connected: boolean = false;

  // Store service methods after client is created
  private registerMethod: any;
  private unregisterMethod: any;
  private heartbeatMethod: any;

  constructor(config: OrchestratorClientConfig = {}) {
    this.config = {
      address: config.address ?? 'localhost:50051',
      useUnixSocket: config.useUnixSocket ?? false,
      credentials: config.credentials ?? credentials.createInsecure(),
      timeout: config.timeout ?? 30000,
    };
  }

  /**
   * Connects to the orchestrator gRPC server
   *
   * @throws Error if connection fails or if already connected
   */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.connected) {
        reject(new Error('Already connected to orchestrator'));
        return;
      }

      try {
        const protoPath = fileURLToPath(new URL('../../../../proto/agent.proto', import.meta.url));
        const protoIncludeDir = path.dirname(protoPath);

        // Load the proto definition dynamically
        // Note: This assumes proto files have been compiled and are available
        const packageDefinition = loadSync(
          protoPath,
          {
            keepCase: false,
            longs: String,
            enums: String,
            defaults: false,
            oneofs: true,
            includeDirs: [protoIncludeDir],
          }
        );

        const protoDescriptor = loadPackageDefinition(packageDefinition);
        const agentService = protoDescriptor.agent.v1.AgentService;

        // Create the gRPC client
        const address = this.config.useUnixSocket
          ? `unix://${this.config.address}`
          : this.config.address;

        this.client = new agentService(
          address,
          this.config.credentials,
          {
            'grpc.max_receive_message_length': -1,
            'grpc.max_send_message_length': -1,
          }
        );

        // Get deadline for connection
        const deadline = new Date();
        deadline.setMilliseconds(deadline.getMilliseconds() + this.config.timeout);

        // Wait for channel to be ready
        this.client.waitForReady(deadline, (err) => {
          if (err) {
            this.client = null;
            this.connected = false;
            reject(new Error(`Failed to connect to orchestrator: ${err.message}`));
            return;
          }

          this.connected = true;

          // Store method references for later use
          this.registerMethod = this.client.register;
          this.unregisterMethod = this.client.unregister;
          this.heartbeatMethod = this.client.heartbeat;

          resolve();
        });
      } catch (err) {
        const error = err as Error;
        reject(new Error(`Failed to create gRPC client: ${error.message}`));
      }
    });
  }

  /**
   * Disconnects from the orchestrator gRPC server
   */
  disconnect(): void {
    if (this.client && this.connected) {
      try {
        this.client.close();
      } catch {
        // Ignore errors during close
      }
      this.connected = false;
      this.client = null;
    }
  }

  /**
   * Checks if the client is connected to the orchestrator
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Registers this agent with the orchestrator
   *
   * @param request - The registration request
   * @returns The registration response containing the agent ID
   * @throws Error if not connected or registration fails
   */
  register(request: RegisterRequest): Promise<RegisterResponse> {
    return new Promise((resolve, reject) => {
      if (!this.connected || !this.client) {
        reject(new Error('Not connected to orchestrator'));
        return;
      }

      const deadline = this.getDeadline();

      this.registerMethod.call(this.client, request, { deadline }, (err: Error | null, response: RegisterResponse) => {
        if (err) {
          reject(new Error(`Registration failed: ${err.message}`));
          return;
        }

        resolve(response);
      });
    });
  }

  /**
   * Unregisters this agent from the orchestrator
   *
   * @param request - The unregistration request
   * @returns The unregistration response
   * @throws Error if not connected or unregistration fails
   */
  unregister(request: UnregisterRequest): Promise<UnregisterResponse> {
    return new Promise((resolve, reject) => {
      if (!this.connected || !this.client) {
        reject(new Error('Not connected to orchestrator'));
        return;
      }

      const deadline = this.getDeadline();

      this.unregisterMethod.call(this.client, request, { deadline }, (err: Error | null, response: UnregisterResponse) => {
        if (err) {
          reject(new Error(`Unregistration failed: ${err.message}`));
          return;
        }

        resolve(response);
      });
    });
  }

  /**
   * Sends a heartbeat to the orchestrator to keep the connection alive
   *
   * @param request - The heartbeat request
   * @returns The heartbeat response with any pending tasks
   * @throws Error if not connected or heartbeat fails
   */
  heartbeat(request: HeartbeatRequest): Promise<HeartbeatResponse> {
    return new Promise((resolve, reject) => {
      if (!this.connected || !this.client) {
        reject(new Error('Not connected to orchestrator'));
        return;
      }

      const deadline = this.getDeadline();

      this.heartbeatMethod.call(this.client, request, { deadline }, (err: Error | null, response: HeartbeatResponse) => {
        if (err) {
          reject(new Error(`Heartbeat failed: ${err.message}`));
          return;
        }

        resolve(response);
      });
    });
  }

  /**
   * Gets the underlying gRPC client for advanced usage
   *
   * @throws Error if not connected
   */
  getRawClient(): Client {
    if (!this.connected || !this.client) {
      throw new Error('Not connected to orchestrator');
    }

    return this.client;
  }

  /**
   * Creates a deadline for the current request based on timeout config
   */
  private getDeadline(): Date {
    const deadline = new Date();
    deadline.setMilliseconds(deadline.getMilliseconds() + this.config.timeout);
    return deadline;
  }
}

/**
 * Creates a new OrchestratorClient instance and connects to the orchestrator
 *
 * @param config - The client configuration
 * @returns A connected OrchestratorClient
 */
export async function createOrchestratorClient(
  config: OrchestratorClientConfig = {}
): Promise<OrchestratorClient> {
  const client = new OrchestratorClient(config);
  await client.connect();
  return client;
}
