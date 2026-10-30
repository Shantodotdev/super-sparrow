import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const base = process.env.SPARROW_E2E_URL;
const chrome = process.env.SPARROW_E2E_CHROME;
if (!base || !chrome) throw new Error("SPARROW_E2E_URL and SPARROW_E2E_CHROME are required");
const profile = await mkdtemp(join(tmpdir(), "sparrow-browser-"));
const browser = Bun.spawn(
  [
    chrome,
    "--headless=new",
    "--no-sandbox",
    "--disable-gpu",
    "--remote-debugging-port=0",
    `--user-data-dir=${profile}`,
    "about:blank",
  ],
  { stdout: "ignore", stderr: "ignore" },
);
let socket: WebSocket | undefined;
let checks = 0;

/** Polls an asynchronous condition with a bounded deadline. */
async function until<T>(check: () => Promise<T>, description: string): Promise<NonNullable<T>> {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    try {
      const result = await check();
      if (result) return result as NonNullable<T>;
    } catch {}
    await Bun.sleep(50);
  }
  throw new Error(`Timed out: ${description}`);
}

try {
  const port = await until(async () => {
    try {
      return (await Bun.file(join(profile, "DevToolsActivePort")).text()).split("\n")[0];
    } catch {
      return undefined;
    }
  }, "start Chromium");
  const tabs = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()) as {
    type: string;
    webSocketDebuggerUrl: string;
  }[];
  socket = new WebSocket(tabs.find((tab) => tab.type === "page")!.webSocketDebuggerUrl);
  const browserErrors: string[] = [];
  const pending = new Map<
    number,
    { resolve: (value: any) => void; reject: (error: Error) => void }
  >();
  let id = 0;
  socket.onmessage = (event) => {
    const message = JSON.parse(String(event.data));
    const task = pending.get(message.id);
    if (!task) {
      if (message.method === "Runtime.exceptionThrown")
        browserErrors.push(message.params.exceptionDetails.text);
      if (message.method === "Runtime.exceptionThrown")
        console.error(
          "Browser error:",
          message.params.exceptionDetails.exception?.description ??
            message.params.exceptionDetails.text,
        );
      if (message.method === "Log.entryAdded" && message.params.entry.level === "error")
        console.error("Browser log:", message.params.entry.text);
      return;
    }
    pending.delete(message.id);
    if (message.error) task.reject(new Error(message.error.message));
    else task.resolve(message.result);
  };
  await new Promise<void>((resolve, reject) => {
    socket!.onopen = () => resolve();
    socket!.onerror = () => reject(new Error("Chromium connection failed"));
  });
  const call = (method: string, params: object) =>
    new Promise<any>((resolve, reject) => {
      pending.set(++id, { resolve, reject });
      socket!.send(JSON.stringify({ id, method, params }));
    });
  const evaluate = async (expression: string) => {
    const result = await call("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true,
    });
    if (result.exceptionDetails)
      throw new Error(
        result.exceptionDetails.exception?.description ?? result.exceptionDetails.text,
      );
    return result.result.value;
  };
  await call("Runtime.enable", {});
  await call("Log.enable", {});
  const waitText = async (text: string) => {
    await until(() => evaluate(`document.body?.innerText.includes(${JSON.stringify(text)})`), text);
    checks++;
    console.log("Verified:", text);
  };
  const click = (text: string) =>
    evaluate(
      `(() => {const button = [...(document.querySelector('[role=dialog]') ?? document).querySelectorAll('button,a')].find(e => e.textContent.trim() === ${JSON.stringify(text)}); if (!button) throw new Error('Missing control'); button.click();})()`,
    );
  const input = (selector: string, value: string) =>
    evaluate(
      `(() => {const e = document.querySelector(${JSON.stringify(selector)}); if (!e) throw new Error('Missing input'); Object.getOwnPropertyDescriptor(e instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype, 'value').set.call(e, ${JSON.stringify(value)}); e.dispatchEvent(new Event('input', {bubbles:true}));})()`,
    );
  const navigate = (path: string) => call("Page.navigate", { url: base + path });
  await navigate("/issues");
  await waitText("Welcome to Super Sparrow").catch(async (error) => {
    console.error(await evaluate("({url: location.href, text: document.body.innerText})"));
    throw error;
  });
  await input('input[type="email"]', "olive@example.com");
  await click("Continue with email");
  await waitText("Check your email");
  const code = await until(async () => {
    const response = await fetch(base + "/__test/code");
    return response.ok ? response.text() : undefined;
  }, "test mailbox");
  await input('input[autocomplete="one-time-code"]', code);
  await click("Sign in");
  await waitText("Meta comments");
  await click("New issue");
  await evaluate(
    `(() => {const e = document.querySelector('select'); e.value = 'WP'; e.dispatchEvent(new Event('change', {bubbles:true}));})()`,
  );
  await input('input[aria-label="Issue title"]', "Browser persisted issue");
  await input(
    'textarea[aria-label="Description or plan"]',
    "## Goal\nBrowser verification\n- [ ] Browser checklist",
  );
  await click("Create issue");
  await waitText("Browser persisted issue");
  await evaluate(`document.querySelector('a[href="/WP-2"]').click()`);
  await waitText("Browser checklist");
  await evaluate(`document.querySelector('input[type="checkbox"]').click()`);
  await until(
    () =>
      evaluate(
        `fetch('/api/issues/WP-2').then(r=>r.json()).then(b=>b.issue.checklist_progress.done === 1)`,
      ),
    "persist checklist",
  );
  checks++;
  await input('input[aria-label="Add a comment"]', "Browser persisted comment");
  await click("Send");
  await waitText("Browser persisted comment");
  await navigate("/WP-2");
  await waitText("Browser persisted comment");
  await click("Edit plan");
  await input(
    'textarea[aria-label="Edit plan"]',
    "## Goal\nUpdated by browser\n- [x] Browser checklist",
  );
  await click("Save plan");
  await waitText("Updated by browser");
  await evaluate(
    `fetch('/api/issues/WP-2', {method:'PATCH', headers:{'Content-Type':'application/json'}, body: JSON.stringify({title:'Live update from another client'})})`,
  );
  await waitText("Live update from another client");
  await navigate("/WP-1/reviews/1");
  await waitText("Browser finding");
  await evaluate(
    `document.activeElement?.blur(); document.body.dispatchEvent(new KeyboardEvent('keydown', {key:'l', bubbles:true}));`,
  );
  await until(
    () =>
      evaluate(
        `fetch('/api/issues/WP-1/reviews/1/findings/1').then(r=>r.json()).then(b=>b.finding.status === 'later' && !!b.finding.follow_up_issue_id)`,
      ),
    "persist finding decision and follow-up",
  );
  checks++;
  await until(
    () =>
      evaluate(
        `Promise.all([fetch('/api/me').then(r=>r.json()), fetch('/api/issues/WP-1/events').then(r=>r.json())]).then(([me, history])=>history.events.some(e=>e.kind === 'finding.status_changed' && e.actor_id === me.actor.id))`,
      ),
    "record signed-in decision actor",
  );
  checks++;

  await navigate("/activity");
  await waitText("Activity");
  await navigate("/settings");
  await waitText("Backups are disabled");
  await click("Create token");
  await waitText("Token label, such as the machine");
  await input("[role=dialog] input", "Browser token");
  await click("Create token");
  await waitText("Copy your token now");
  await click("Done");
  await waitText("Browser token");
  await until(() => evaluate(`!document.querySelector('[role=dialog]')`), "close token dialog");
  await click("Renew");
  await until(
    () =>
      evaluate(
        `fetch('/api/tokens').then(r=>r.json()).then(b=>b.tokens.some(t=>t.label === 'Browser token'))`,
      ),
    "renew token",
  );
  checks++;
  await click("Revoke");
  await waitText("Revoke “Browser token”?");
  await click("Revoke token");
  await until(
    () =>
      evaluate(
        `fetch('/api/tokens').then(r=>r.json()).then(b=>!b.tokens.some(t=>t.label === 'Browser token'))`,
      ),
    "revoke token",
  );
  checks++;
  await navigate("/settings/sessions");
  await waitText("Current");
  await click("Sign out all devices");
  await waitText("Welcome to Super Sparrow");
  if (browserErrors.length) throw new Error(`${browserErrors.length} browser runtime errors`);
  console.log(`${checks} browser assertions passed`);
} finally {
  socket?.close();
  browser.kill();
  await browser.exited;
  await rm(profile, { recursive: true, force: true });
}
