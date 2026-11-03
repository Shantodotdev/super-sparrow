import type { ReactNode } from "react";
import { useRouterState } from "@tanstack/react-router";
import { Sidebar } from "./sidebar";

/** Keeps navigation and page geometry independent of resource loading and background refreshes. */
export function AppFrame({ children }: { children: ReactNode }) {
  // The URL of the page this frame shows; it lags the address bar while a navigation is loading.
  const renderedPath = useRouterState({ select: (state) => state.resolvedLocation?.pathname });

  return (
    <div data-page-cache data-page-path={renderedPath} className="flex min-h-screen">
      <Sidebar />
      <main className="min-w-0 flex-1 [&:has(>_.issue-board)]:flex [&:has(>_.issue-board)]:h-dvh [&:has(>_.issue-board)]:min-h-0 [&:has(>_.issue-board)]:flex-col [&:has(>_.issue-board)]:overflow-hidden">
        {children}
      </main>
    </div>
  );
}
