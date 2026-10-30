import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../lib/api";
import { ProjectOnboarding } from "./project-onboarding";
import { meQuery, projectsQuery } from "../queries/project";
import { useEffect, useState } from "react";
import { Outlet, useMatches } from "@tanstack/react-router";
import { CommandPalette } from "./command-palette";
import { Sidebar } from "./sidebar";

/**
 * The frame around every signed-in page: the sidebar, the page itself, and the command palette.
 * A route whose static data sets `bare` renders on its own, without the frame.
 */
export function AppShell() {
  const bare = useMatches({ select: (matches) => matches.some((match) => match.staticData.bare) });
  const client = useQueryClient();
  const projects = useQuery({ ...projectsQuery, enabled: !bare });
  const identity = useQuery({ ...meQuery, enabled: !bare, refetchInterval: 30_000 });
  useEffect(() => {
    if (identity.error instanceof ApiError && identity.error.status === 401) {
      client.clear();
      window.location.assign("/signin");
    }
  }, [identity.error, client]);
  useEffect(() => {
    if (bare) return;
    let stream: EventSource;
    const connect = () => {
      stream = new EventSource("/api/events/stream");
      stream.addEventListener("change", () => {
        void client.invalidateQueries();
      });
      stream.onopen = () => {
        void client.invalidateQueries();
      };
    };
    const close = () => stream.close();
    const resume = (event: PageTransitionEvent) => {
      if (event.persisted) connect();
    };
    connect();
    window.addEventListener("pagehide", close);
    window.addEventListener("pageshow", resume);
    return () => {
      close();
      window.removeEventListener("pagehide", close);
      window.removeEventListener("pageshow", resume);
    };
  }, [bare, client]);
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    const openOnShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen(true);
      }
    };

    window.addEventListener("keydown", openOnShortcut);

    return () => window.removeEventListener("keydown", openOnShortcut);
  }, []);

  if (bare) return <Outlet />;
  if (projects.isPending)
    return (
      <p role="status" className="p-8">
        Loading projects…
      </p>
    );
  if (projects.error)
    return (
      <div role="alert" className="p-8">
        <p>{projects.error.message}</p>
        <button onClick={() => void projects.refetch()}>Try again</button>
      </div>
    );
  if (!projects.data.length) return <ProjectOnboarding />;

  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="min-w-0 flex-1 [&:has(>_.issue-board)]:flex [&:has(>_.issue-board)]:h-dvh [&:has(>_.issue-board)]:min-h-0 [&:has(>_.issue-board)]:flex-col [&:has(>_.issue-board)]:overflow-hidden">
        <Outlet />
      </main>
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
    </div>
  );
}
