import { createFileRoute } from "@tanstack/react-router";
import { PageView } from "../app";

export const Route = createFileRoute("/signin")({ component: () => <PageView page="signin" /> });
