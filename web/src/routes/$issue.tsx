import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/$issue")({ component: () => <PageView page="issue" /> });
