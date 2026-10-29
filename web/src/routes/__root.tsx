import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { HeadContent, Scripts, createRootRouteWithContext } from "@tanstack/react-router";
import appCss from "../styles.css?url";
import { AppShell } from "../components/app-shell";
import { ToastProvider } from "../components/toast";

/** The HTML shell around every page, with the shared query client from the router context. */
export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
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
    <html lang="en" className="app-scrollbar">
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

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AppShell />
      </ToastProvider>
    </QueryClientProvider>
  );
}
