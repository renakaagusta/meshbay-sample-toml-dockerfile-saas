import type { Metadata } from "next";

import "./globals.css";

export const metadata: Metadata = {
  title: "meshbay-sample-saas",
  description: "Meshbay fixture: one repo, web + api + worker, Postgres and Redis",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
