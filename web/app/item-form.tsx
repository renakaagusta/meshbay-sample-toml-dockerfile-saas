"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

export function ItemForm() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) {
      return;
    }
    setPending(true);
    setMessage(null);
    try {
      const response = await fetch("/api/items", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name }),
      });
      const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      if (response.ok) {
        setMessage(`queued item ${payload.id} (${payload.status})`);
        setName("");
        router.refresh();
      } else {
        setMessage(`failed: ${payload.error ?? `HTTP ${response.status}`}`);
      }
    } catch (error) {
      setMessage(`failed: ${error instanceof Error ? error.message : "unknown error"}`);
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <form onSubmit={submit}>
        <input
          type="text"
          name="name"
          value={name}
          placeholder="Item name"
          aria-label="Item name"
          onChange={(event) => setName(event.target.value)}
        />
        <button type="submit" disabled={pending || name.trim() === ""}>
          {pending ? "Queueing…" : "Add item"}
        </button>
      </form>
      {message ? <p className="message">{message}</p> : null}
    </>
  );
}
