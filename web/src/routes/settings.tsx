import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/settings")({
  component: () => <PageView page="settings" />,
});
