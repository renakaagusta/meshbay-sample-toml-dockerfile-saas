import { forward } from "@/lib/api";

export const dynamic = "force-dynamic";

export async function GET() {
  return forward("/api/stats");
}
