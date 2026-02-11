// Entry point for the LLM Gateway service.

import { loadConfig } from "./config";
import { log } from "./logger";
import { Gateway } from "./server";

async function main(): Promise<void> {
  log("info", "Starting LLM Gateway service");

  const config = loadConfig();

  log("info", "Configuration loaded", {
    port: config.port,
    default_provider: config.defaultProvider,
    default_model: config.defaultModel,
    timeout_ms: config.timeout,
    max_concurrent: config.maxConcurrentRequests,
    providers: config.providers.map((p) => p.name),
  });

  const gateway = new Gateway(config);
  await gateway.start();

  // Graceful shutdown
  const shutdown = async (signal: string) => {
    log("info", `Received ${signal}, shutting down gracefully`);
    await gateway.stop();
    process.exit(0);
  };

  process.on("SIGTERM", () => shutdown("SIGTERM"));
  process.on("SIGINT", () => shutdown("SIGINT"));
}

main().catch((err) => {
  log("error", "Fatal error starting gateway", { error: String(err) });
  process.exit(1);
});
