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

  return (
    <div className="app-shell">
      <Sidebar />
      <main className="main-area">
        <Outlet />
      </main>
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
    </div>
  );
}
