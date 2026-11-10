type LegacyRegistration = {
  active: { scriptURL: string } | null;
  waiting: { scriptURL: string } | null;
  installing: { scriptURL: string } | null;
  unregister: () => Promise<boolean>;
};

type WorkerRegistry = { getRegistrations: () => Promise<readonly LegacyRegistration[]> };

/** Removes the former app worker from existing browsers without affecting other registrations. */
export async function retireServiceWorker(registry: WorkerRegistry, origin: string): Promise<void> {
  const scriptURL = new URL("/sw.js", origin).href;
  const registrations = await registry.getRegistrations();
  await Promise.all(
    registrations
      .filter((registration) =>
        [registration.active, registration.waiting, registration.installing].some(
          (worker) => worker?.scriptURL === scriptURL,
        ),
      )
      .map((registration) => registration.unregister()),
  );
}
