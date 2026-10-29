import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/activity")({
  component: () => <PageView page="activity" />,
});
