import { ItemForm } from "./item-form";
import { apiBaseUrl, listItems, loadStats, type Item, type Stats } from "@/lib/api";

// API_URL is read per request, never at build time.
export const dynamic = "force-dynamic";

export default async function Page() {
  let items: Item[] = [];
  let stats: Stats = { queued: 0, processed: 0, total: 0 };
  let failure: string | null = null;

  try {
    [items, stats] = await Promise.all([listItems(), loadStats()]);
  } catch (error) {
    failure = error instanceof Error ? error.message : "unknown error";
  }

  return (
    <main>
      <h1>meshbay-sample-saas</h1>
      <p className="lead">
        Next.js web → Go api → Redis → Node worker → Postgres. Reading <code>API_URL</code> ={" "}
        <code>{apiBaseUrl()}</code> at runtime.
      </p>

      <section>
        <h2>Queue</h2>
        <div className="stats">
          <div>
            <strong>{stats.total}</strong>
            total
          </div>
          <div>
            <strong>{stats.queued}</strong>
            queued
          </div>
          <div>
            <strong>{stats.processed}</strong>
            processed
          </div>
        </div>
      </section>

      <section>
        <h2>New item</h2>
        <ItemForm />
      </section>

      <section>
        <h2>Items</h2>
        {failure ? (
          <p className="message error">api unreachable: {failure}</p>
        ) : items.length === 0 ? (
          <p className="message">No items yet — add one above and watch the worker mark it processed.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>id</th>
                <th>name</th>
                <th>status</th>
                <th>created</th>
                <th>processed</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td>{item.id}</td>
                  <td>{item.name}</td>
                  <td>
                    <span className={`badge ${item.status}`}>{item.status}</span>
                  </td>
                  <td>{item.created_at}</td>
                  <td>{item.processed_at ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
