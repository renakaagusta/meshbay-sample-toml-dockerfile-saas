// Consumes the Redis queue the api fills and marks each item processed through
// the api's private address (API_URL). One log line per job.
import Redis from "ioredis";

import { resolveApiUrl } from "./api-url.js";

const queueName = process.env.QUEUE_NAME || "jobs";
const apiUrl = resolveApiUrl();
const maximumAttempts = 5;
const blockSeconds = 5;

const redis = new Redis(process.env.REDIS_URL, {
  maxRetriesPerRequest: null,
  lazyConnect: false,
});

let stopping = false;

function log(message) {
  console.log(`${new Date().toISOString()} worker: ${message}`);
}

function sleep(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function markProcessed(job) {
  let lastError = null;
  for (let attempt = 1; attempt <= maximumAttempts; attempt += 1) {
    try {
      const response = await fetch(`${apiUrl}/api/items/${job.id}/process`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ worker: process.env.HOSTNAME || "worker" }),
        signal: AbortSignal.timeout(10000),
      });
      if (response.status === 200) {
        const item = await response.json();
        log(`job item=${job.id} name=${JSON.stringify(job.name)} status=${item.status} attempt=${attempt}`);
        return;
      }
      if (response.status === 404) {
        log(`job item=${job.id} dropped: the item no longer exists`);
        return;
      }
      lastError = new Error(`api answered HTTP ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    if (attempt < maximumAttempts) {
      await sleep(1000 * 2 ** (attempt - 1));
    }
  }
  // Never lose a job: put it back at the tail and let another attempt pick it up.
  await redis.rpush(queueName, JSON.stringify(job));
  log(`job item=${job.id} requeued after ${maximumAttempts} attempts: ${lastError?.message ?? "unknown error"}`);
}

async function handle(rawJob) {
  let job;
  try {
    job = JSON.parse(rawJob);
  } catch (error) {
    log(`dropped malformed job: ${error.message}`);
    return;
  }
  if (!job || typeof job.id !== "number") {
    log("dropped job without a numeric id");
    return;
  }
  await markProcessed(job);
}

async function main() {
  log(`starting api=${apiUrl} queue=${queueName}`);
  redis.on("error", (error) => log(`redis: ${error.message}`));
  while (!stopping) {
    const entry = await redis.brpop(queueName, blockSeconds);
    if (!entry) {
      continue;
    }
    await handle(entry[1]);
  }
  await redis.quit();
  log("stopped");
}

for (const signal of ["SIGTERM", "SIGINT"]) {
  process.on(signal, () => {
    log(`received ${signal}, finishing the current job`);
    stopping = true;
  });
}

main().catch((error) => {
  log(`fatal: ${error.stack ?? error.message}`);
  process.exit(1);
});
