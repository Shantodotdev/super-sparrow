import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/findings")({
  component: () => <PageView page="findings" />,
});
