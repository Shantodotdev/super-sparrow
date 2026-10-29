import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/$issue_/reviews/$round")({
  component: () => <PageView page="review" />,
});
