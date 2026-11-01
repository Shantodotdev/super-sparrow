import { useEffect, useState } from "react";
import { ApiError } from "../lib/api";
import { meQuery } from "../queries/project";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { redirect, HeadContent, Scripts, createRootRouteWithContext } from "@tanstack/react-router";
import appCss from "../styles.css?url";
import { AppShell } from "../components/app-shell";
import { ToastProvider } from "../components/toast";
import { clearQueryCache } from "../queries/persistence";

/** The HTML shell around every page, with the shared query client from the router context. */
export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  pendingComponent: () => null,
  pendingMs: 0,
  pendingMinMs: 0,
  beforeLoad: async ({ context, location }) => {
    if (typeof window === "undefined") return;
    if (location.pathname.replace(/\/+$/, "") === "/signin") {
      clearQueryCache(context.queryClient);
      return;
    }

    if (context.queryClient.getQueryData(meQuery.queryKey)) {
      void context.queryClient.prefetchQuery(meQuery);
      return;
    }

    try {
      await context.queryClient.fetchQuery({ ...meQuery, staleTime: 0 });
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        clearQueryCache(context.queryClient);
        throw redirect({ to: "/signin" });
      }
      throw error;
    }
  },
  errorComponent: ({ error }) => (
    <div role="alert" className="p-8">
      <p>{error instanceof Error ? error.message : "Could not load the page"}</p>
      <a href="/signin">Sign in again</a>
    </div>
  ),
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: "Super Sparrow" },
    ],
    links: [{ rel: "stylesheet", href: appCss }],
  }),
  shellComponent: RootDocument,
  component: RootLayout,
});

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="scrollbar-thin [scrollbar-color:#465062_transparent]">
      <head>
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  );
}

function RootLayout() {
  const { queryClient } = Route.useRouteContext();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>{mounted ? <AppShell /> : null}</ToastProvider>
    </QueryClientProvider>
  );
}
