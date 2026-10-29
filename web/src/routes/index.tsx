import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/")({
  validateSearch: (search: Record<string, unknown>): { view?: "board" } => ({
    view: search.view === "board" ? "board" : undefined,
  }),
  component: () => <PageView page="issues" />,
});
