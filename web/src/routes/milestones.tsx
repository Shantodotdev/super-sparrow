import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/milestones")({
  component: () => <PageView page="milestones" />,
});
