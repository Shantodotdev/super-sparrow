import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { apiGet } from "../lib/api";

export const Route = createFileRoute("/")({
  component: HomePage,
});

function HomePage() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: () => apiGet<{ status: string }>("/api/health"),
  });

  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col justify-center gap-2 px-6">
      <h1 className="text-2xl font-semibold">Super Sparrow</h1>
      <p className="text-neutral-600">
        Server:{" "}
        {health.isPending ? "checking…" : health.isError ? "unreachable" : health.data.status}
      </p>
    </main>
  );
}
