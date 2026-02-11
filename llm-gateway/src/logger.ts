// Structured JSON logger that writes to stdout (read-only rootfs: no log files).

export type LogLevel = "debug" | "info" | "warn" | "error";

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

const minLevel: LogLevel =
  (process.env.LOG_LEVEL as LogLevel) || "info";

export function log(
  level: LogLevel,
  msg: string,
  fields?: Record<string, unknown>
): void {
  if (LOG_LEVELS[level] < LOG_LEVELS[minLevel]) return;

  const entry: Record<string, unknown> = {
    time: new Date().toISOString(),
    level: level.toUpperCase(),
    msg,
    component: "llm-gateway",
    ...fields,
  };

  const out = level === "error" ? process.stderr : process.stdout;
  out.write(JSON.stringify(entry) + "\n");
}
