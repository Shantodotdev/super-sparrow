import { ProjectForm } from "./project-form";

/** Gives an empty team one focused starting point without application navigation. */
export function ProjectOnboarding() {
  return (
    <main className="relative isolate grid min-h-dvh place-items-center overflow-hidden bg-[#08090b] px-6 py-12">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -top-64 left-1/2 -z-10 h-[650px] w-[850px] -translate-x-1/2 rounded-full bg-[#2455eb]/10 blur-[120px]"
      />
      <section className="w-full max-w-[520px]">
        <img src="/logo.png" alt="Super Sparrow" className="mb-8 h-28 w-28" />
        <h1 className="mb-8 text-4xl leading-tight tracking-tight text-[#f1f4fa]">
          Create your first project
        </h1>
        <div className="rounded-2xl border border-[#283142] bg-[#10141c]/90 p-7 shadow-[0_24px_80px_#0005]">
          <ProjectForm />
        </div>
      </section>
    </main>
  );
}
