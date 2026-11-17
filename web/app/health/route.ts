import { apiBaseUrl } from "@/lib/api";

export const dynamic = "force-dynamic";

/**
 * Healthcheck of the web service. It also pings the api, so a green web probe
 * means the private service network works (web -> api), which is half of the
 * fixture. A red api makes this endpoint 502 on purpose.
 */
export async function GET() {
  try {
    const response = await fetch(`${apiBaseUrl()}/health`, {
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
    });
    const payload = await response.json().catch(() => ({ status: `HTTP ${response.status}` }));
    if (!response.ok) {
      return Response.json({ status: "degraded", api: payload }, { status: 502 });
    }
    return Response.json({ status: "ok", api: payload });
  } catch (error) {
    return Response.json(
      {
        status: "degraded",
        api: error instanceof Error ? error.message : "unreachable",
      },
      { status: 502 },
    );
  }
}
