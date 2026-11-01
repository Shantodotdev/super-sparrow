import { refreshLiveEvents } from "../queries/live-events";
import type { ActivityEvent } from "../lib/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../lib/api";
import { ProjectOnboarding } from "./project-onboarding";
import { meQuery, projectsQuery } from "../queries/project";
import { useEffect, useState } from "react";
import { Outlet, useMatches } from "@tanstack/react-router";
import { CommandPalette } from "./command-palette";
import { AppFrame } from "./app-frame";
import { clearQueryCache } from "../queries/persistence";

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
      clearQueryCache(client);
      window.location.assign("/signin");
    }
  }, [identity.error, client]);
  useEffect(() => {
    if (bare) return;
    let stream: EventSource;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const changes: ActivityEvent[] = [];
    const connect = () => {
      stream = new EventSource("/api/events/stream");
      stream.addEventListener("change", (message: MessageEvent<string>) => {
        try {
          const event = JSON.parse(message.data) as ActivityEvent;
          if (!event || typeof event.target_type !== "string" || !event.payload) return;
          changes.push(event);
          timer ??= setTimeout(() => {
            timer = undefined;
            refreshLiveEvents(client, changes.splice(0));
          }, 100);
        } catch {
          // Ignore malformed events; reconnecting refreshes the cached state.
        }
      });
      stream.onopen = () => {
        void client.invalidateQueries({}, { cancelRefetch: false });
      };
    };
    const close = () => {
      stream.close();
      clearTimeout(timer);
      timer = undefined;
      changes.length = 0;
    };
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
  if (!identity.data) return null;
  if (projects.data?.length === 0) return <ProjectOnboarding />;

  return (
    <AppFrame>
      {projects.error && (
        <div role="alert" className="px-8 py-4 text-[#f38b8b]">
          <p>{projects.error.message}</p>
          <button onClick={() => void projects.refetch()}>Try again</button>
        </div>
      )}
      {!!projects.data?.length && <Outlet />}
      {!!projects.data?.length && (
        <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
      )}
    </AppFrame>
  );
}
