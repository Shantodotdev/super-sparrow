import { useEffect, useState } from "react";
import { ApiError } from "../lib/api";
import { meQuery } from "../queries/project";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import {
  redirect,
  HeadContent,
  Scripts,
  createRootRouteWithContext,
  useRouterState,
  type ErrorComponentProps,
} from "@tanstack/react-router";
import appCss from "../styles.css?url";
import { AppShell } from "../components/app-shell";
import { ToastProvider } from "../components/toast";
import { clearQueryCache } from "../queries/persistence";
import previewScript from "../lib/page-preview-runtime.js?url";
import { dismissPagePreview, watchPagePreview } from "../lib/page-preview";

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
  errorComponent: RootError,
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: "Super Sparrow" },
    ],
    links: [
      { rel: "stylesheet", href: appCss },
      { rel: "preload", href: previewScript, as: "script" },
    ],
  }),
  shellComponent: RootDocument,
  component: RootLayout,
});

/** Makes startup errors visible and interactive even when a cached preview was restored. */
function RootError({ error }: ErrorComponentProps) {
  useEffect(dismissPagePreview, []);

  return (
    <div role="alert" className="p-8">
      <p>{error instanceof Error ? error.message : "Could not load the page"}</p>
      <a href="/signin">Sign in again</a>
    </div>
  );
}

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="scrollbar-thin [scrollbar-color:#465062_transparent]">
      <head>
        <HeadContent />
      </head>
      <body>
        <div id="page-preview" aria-hidden="true" inert suppressHydrationWarning />
        <div id="app-content" suppressHydrationWarning>
          {children}
        </div>
        <script src={previewScript} />
        <Scripts />
      </body>
    </html>
  );
}

function RootLayout() {
  const { queryClient } = Route.useRouteContext();
  const loading = useRouterState({ select: (state) => state.isLoading });
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  useEffect(() => {
    if (mounted && !loading) return watchPagePreview(queryClient);
  }, [mounted, loading, queryClient]);

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>{mounted ? <AppShell /> : null}</ToastProvider>
    </QueryClientProvider>
  );
}
