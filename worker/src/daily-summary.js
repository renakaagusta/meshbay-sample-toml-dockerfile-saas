// Cron command registered in Meshbay against the worker service:
//
//   node worker/src/daily-summary.js
//
// It prints one summary line and exits, which is what a Meshbay cron run shows
// in its log summary.
import { resolveApiUrl } from "./api-url.js";

const apiUrl = resolveApiUrl();

async function readJSON(path) {
  const response = await fetch(`${apiUrl}${path}`, { signal: AbortSignal.timeout(10000) });
  if (!response.ok) {
    throw new Error(`${path} answered HTTP ${response.status}`);
  }
  return response.json();
}

try {
  const [stats, items] = await Promise.all([readJSON("/api/stats"), readJSON("/api/items")]);
  const queued = items.filter((item) => item.status === "queued");
  const oldestQueued = queued.length > 0 ? `oldest_queued_id=${queued[0].id}` : "oldest_queued_id=none";
  console.log(
    `daily-summary at=${new Date().toISOString()} total=${stats.total} queued=${stats.queued} ` +
      `processed=${stats.processed} ${oldestQueued}`,
  );
  process.exit(0);
} catch (error) {
  console.error(`daily-summary failed against ${apiUrl}: ${error.message}`);
  process.exit(1);
}
