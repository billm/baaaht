// Configuration loaded from environment variables injected by the orchestrator.

export interface GatewayConfig {
  port: number;
  defaultProvider: string;
  defaultModel: string;
  timeout: number; // milliseconds
  maxConcurrentRequests: number;
  providers: ProviderConfig[];
}

export interface ProviderConfig {
  name: string;
  apiKey: string;
  baseUrl?: string;
}

/**
 * Parse a Go duration string (e.g., "2m0s", "120s", "1h30m") into milliseconds.
 */
function parseGoDuration(duration: string): number {
  let total = 0;
  const hourMatch = duration.match(/(\d+)h/);
  const minMatch = duration.match(/(\d+)m(?!s)/);
  const secMatch = duration.match(/([\d.]+)s/);

  if (hourMatch) total += parseInt(hourMatch[1], 10) * 3600000;
  if (minMatch) total += parseInt(minMatch[1], 10) * 60000;
  if (secMatch) total += parseFloat(secMatch[1]) * 1000;

  return total || 120000; // default 2 minutes
}

export function loadConfig(): GatewayConfig {
  const providers: ProviderConfig[] = [];

  // Collect provider credentials from environment
  const anthropicKey = process.env.ANTHROPIC_API_KEY;
  if (anthropicKey) {
    providers.push({
      name: "anthropic",
      apiKey: anthropicKey,
      baseUrl: process.env.ANTHROPIC_BASE_URL || "https://api.anthropic.com",
    });
  }

  const openaiKey = process.env.OPENAI_API_KEY;
  if (openaiKey) {
    providers.push({
      name: "openai",
      apiKey: openaiKey,
      baseUrl: process.env.OPENAI_BASE_URL || "https://api.openai.com/v1",
    });
  }

  const azureKey = process.env.AZURE_API_KEY;
  if (azureKey) {
    providers.push({
      name: "azure",
      apiKey: azureKey,
      baseUrl: process.env.AZURE_BASE_URL,
    });
  }

  const googleKey = process.env.GOOGLE_API_KEY;
  if (googleKey) {
    providers.push({
      name: "google",
      apiKey: googleKey,
      baseUrl: process.env.GOOGLE_BASE_URL,
    });
  }

  const timeoutStr = process.env.LLM_TIMEOUT || "2m0s";

  return {
    port: parseInt(process.env.PORT || "8080", 10),
    defaultProvider: process.env.LLM_DEFAULT_PROVIDER || "anthropic",
    defaultModel: process.env.LLM_DEFAULT_MODEL || "anthropic/claude-sonnet-4-20250514",
    timeout: parseGoDuration(timeoutStr),
    maxConcurrentRequests: parseInt(process.env.LLM_MAX_CONCURRENT_REQUESTS || "10", 10),
    providers,
  };
}

export function getProviderConfig(
  config: GatewayConfig,
  providerName: string
): ProviderConfig | undefined {
  return config.providers.find((p) => p.name === providerName);
}
