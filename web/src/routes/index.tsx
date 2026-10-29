import { createFileRoute, redirect } from "@tanstack/react-router";
import { validateIssuesSearch } from "../lib/search";

export const Route = createFileRoute("/")({
  validateSearch: validateIssuesSearch,
  beforeLoad: ({ search }) => {
    throw redirect({ to: "/issues", search });
  },
});
