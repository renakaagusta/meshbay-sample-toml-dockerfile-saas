import { forward } from "@/lib/api";

export const dynamic = "force-dynamic";

type Context = { params: Promise<{ itemID: string }> };

export async function GET(_request: Request, context: Context) {
  const { itemID } = await context.params;
  return forward(`/api/items/${encodeURIComponent(itemID)}`);
}

export async function PUT(request: Request, context: Context) {
  const { itemID } = await context.params;
  return forward(`/api/items/${encodeURIComponent(itemID)}`, {
    method: "PUT",
    body: await request.text(),
  });
}

export async function DELETE(_request: Request, context: Context) {
  const { itemID } = await context.params;
  return forward(`/api/items/${encodeURIComponent(itemID)}`, { method: "DELETE" });
}
