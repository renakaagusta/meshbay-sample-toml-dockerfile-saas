import { forward } from "@/lib/api";

export const dynamic = "force-dynamic";

export async function GET() {
  return forward("/api/items");
}

export async function POST(request: Request) {
  const body = await request.text();
  return forward("/api/items", { method: "POST", body });
}
