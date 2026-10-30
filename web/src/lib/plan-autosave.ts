import { ApiError } from "./api";

export type PlanVersion = { plan: string; revision: number };
export type PlanSaveState = PlanVersion & { dirty: boolean; saving: boolean; error: Error | null };

type Options = {
  initial: PlanVersion;
  draft?: PlanVersion;
  save: (value: PlanVersion) => Promise<PlanVersion>;
  persist: (draft: PlanVersion | null) => void;
  delay?: number;
};

/** Serializes debounced plan writes, preserving newer typing and stopping on revision conflicts. */
export class PlanAutosave {
  private saved: string;
  private plan: string;
  private revision: number;
  private error: Error | null = null;
  private running = false;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private listener: ((state: PlanSaveState) => void) | undefined;

  constructor(private readonly options: Options) {
    this.saved = options.initial.plan;
    this.plan = options.draft?.plan ?? this.saved;
    this.revision = options.draft?.revision ?? options.initial.revision;
  }

  get state(): PlanSaveState {
    return {
      plan: this.plan,
      revision: this.revision,
      dirty: this.plan !== this.saved,
      saving: this.running,
      error: this.error,
    };
  }

  subscribe(listener: (state: PlanSaveState) => void): () => void {
    this.listener = listener;
    listener(this.state);
    if (this.state.dirty) this.schedule();
    return () => {
      this.listener = undefined;
    };
  }

  edit(plan: string): void {
    this.plan = plan;
    if (!(this.error instanceof ApiError && this.error.status === 409)) this.error = null;
    this.publish();
    if (!this.error) this.schedule();
  }

  /** Accepts remote revisions only when there are no local edits or writes in flight. */
  receive(value: PlanVersion): boolean {
    if (this.running || this.state.dirty || value.revision <= this.revision) return false;
    this.saved = this.plan = value.plan;
    this.revision = value.revision;
    this.error = null;
    this.publish();
    return true;
  }

  /** Explicitly discards the draft in favor of a freshly loaded server version. */
  reset(value: PlanVersion): void {
    clearTimeout(this.timer);
    this.saved = this.plan = value.plan;
    this.revision = value.revision;
    this.error = null;
    this.publish();
  }

  async flush(): Promise<void> {
    clearTimeout(this.timer);
    if (
      this.running ||
      !this.state.dirty ||
      (this.error instanceof ApiError && this.error.status === 409)
    )
      return;
    this.running = true;
    this.publish();
    const sent = { plan: this.plan, revision: this.revision };
    try {
      const saved = await this.options.save(sent);
      this.saved = saved.plan;
      this.revision = saved.revision;
      if (this.plan === sent.plan) this.plan = saved.plan;
      this.error = null;
    } catch (error) {
      this.error = error instanceof Error ? error : new Error("Could not save the plan");
    } finally {
      this.running = false;
      this.publish();
      if (
        this.state.dirty &&
        (!this.error ||
          !(this.error instanceof ApiError) ||
          this.error.status >= 500 ||
          this.error.status === 408 ||
          this.error.status === 429)
      ) {
        this.schedule(this.error ? 3000 : undefined);
      }
    }
  }

  private schedule(delay = this.options.delay ?? 600): void {
    clearTimeout(this.timer);
    this.timer = setTimeout(() => void this.flush(), delay);
  }

  private publish(): void {
    this.options.persist(
      this.state.dirty || this.running ? { plan: this.plan, revision: this.revision } : null,
    );
    this.listener?.(this.state);
  }
}
