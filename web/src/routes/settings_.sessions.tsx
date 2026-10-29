import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/settings_/sessions")({
  component: () => <PageView page="sessions" />,
});
