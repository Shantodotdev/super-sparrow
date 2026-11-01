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
  const requests = new Map<string, string>();
  const pending = new Map<
    number,
    { resolve: (value: any) => void; reject: (error: Error) => void }
  >();
  let id = 0;
  socket.onmessage = (event) => {
    const message = JSON.parse(String(event.data));
    const task = pending.get(message.id);
    if (!task) {
      if (message.method === "Network.requestWillBeSent")
        requests.set(message.params.requestId, message.params.request.url);
      if (
        message.method === "Network.loadingFinished" ||
        message.method === "Network.loadingFailed"
      )
        requests.delete(message.params.requestId);
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
  const screenshot = async (name: string) => {
    const directory = process.env.SPARROW_E2E_SCREENSHOT_DIR;
    if (!directory) return;
    await evaluate(
      `Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))`,
    );
    const { data } = await call("Page.captureScreenshot", { format: "png" });
    await Bun.write(join(directory, `${name}.png`), Buffer.from(data, "base64"));
  };
  if (process.env.SPARROW_E2E_SCREENSHOT_DIR) {
    await call("Emulation.setDeviceMetricsOverride", {
      width: 1440,
      height: 1000,
      deviceScaleFactor: 1,
      mobile: false,
    });
  }
  await call("Page.bringToFront", {});
  await call("Runtime.enable", {});
  await call("Network.enable", {});
  await call("Log.enable", {});
  const waitText = async (text: string) => {
    try {
      await until(
        () => evaluate(`document.body?.innerText.includes(${JSON.stringify(text)})`),
        text,
      );
    } catch (error) {
      console.error(await evaluate("({url: location.href, text: document.body.innerText})"));
      console.error("Pending requests:", [...requests.values()]);
      throw error;
    }
    checks++;
    console.log("Verified:", text);
  };
  const click = async (text: string) => {
    await until(
      () =>
        evaluate(
          `!![...(document.querySelector('[role=dialog]') ?? document).querySelectorAll('button,a')].find(e=>e.textContent.trim() === ${JSON.stringify(text)})`,
        ),
      `find control ${text}`,
    );
    await evaluate(
      `(() => {const button = [...(document.querySelector('[role=dialog]') ?? document).querySelectorAll('button,a')].find(e => e.textContent.trim() === ${JSON.stringify(text)}); if (!button) throw new Error('Missing control'); button.click();})()`,
    );
  };
  const checkStatusMenu = async (label: string, name: string) => {
    const selector = `[role="dialog"] [role="combobox"][aria-label="${label}"]`;
    await evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
    await until(
      () => evaluate(`!!document.querySelector('[role="listbox"] [role="option"]')`),
      "open status menu",
    );
    await evaluate(
      `Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))`,
    );
    await screenshot(name);
    if (
      !(await evaluate(`(() => {
      const dialog = document.querySelector('[role="dialog"]');
      const menu = document.querySelector('[role="listbox"]');
      const rect = menu.getBoundingClientRect();
      const visible = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      const trigger = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();
      const expectedLeft = Math.max(12, Math.min(trigger.left, innerWidth - 12 - rect.width));
      return Math.abs(rect.left - expectedLeft) < 1 && menu.matches(":popover-open") && rect.top >= 0 && rect.bottom <= innerHeight && menu.contains(visible);
    })()`))
    )
      throw new Error("Status menu must be visible and clickable without scrolling the form");
    checks++;
    const formScroll = await evaluate(`document.querySelector('[role="dialog"]').scrollTop`);
    await evaluate(
      `document.querySelector(${JSON.stringify(selector)}).dispatchEvent(new KeyboardEvent('keydown', {key:'End', bubbles:true}))`,
    );
    await until(
      () =>
        evaluate(
          `document.querySelector(${JSON.stringify(selector)}).getAttribute('aria-activedescendant') === document.querySelector('[role="listbox"] [role="option"]:last-child').id`,
        ),
      "navigate to the last selector option",
    );
    if ((await evaluate(`document.querySelector('[role="dialog"]').scrollTop`)) !== formScroll)
      throw new Error("Keyboard selector navigation scrolled the form");
    checks++;
    await evaluate(
      `document.querySelector(${JSON.stringify(selector)}).dispatchEvent(new KeyboardEvent('keydown', {key:'Escape', bubbles:true}))`,
    );
    if (
      !(await evaluate(
        `!!document.querySelector('[role="dialog"]') && !document.querySelector('[role="listbox"]')`,
      ))
    )
      throw new Error("Escape should close the status menu and leave the form open");
    checks++;
  };
  const input = async (selector: string, value: string) => {
    await until(
      () => evaluate(`!!document.querySelector(${JSON.stringify(selector)})`),
      `find input ${selector}`,
    );
    return evaluate(
      `(() => {const e = document.querySelector(${JSON.stringify(selector)}); if (!e) throw new Error('Missing input'); Object.getOwnPropertyDescriptor(e instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype, 'value').set.call(e, ${JSON.stringify(value)}); e.dispatchEvent(new Event('input', {bubbles:true}));})()`,
    );
  };
  const navigate = async (path: string, destination = path) => {
    await call("Page.navigate", { url: base + path });
    await until(
      () =>
        evaluate(
          `location.pathname === ${JSON.stringify(destination)} && document.readyState === 'complete'`,
        ),
      `load ${path}`,
    );
  };
  await navigate("/", "/signin");
  await waitText("Sign in").catch(async (error) => {
    console.error(await evaluate("({url: location.href, text: document.body.innerText})"));
    throw error;
  });
  await input('input[type="email"]', "olive@example.com");
  await click("Continue");
  await waitText("Enter your code");
  const code = await until(async () => {
    const response = await fetch(base + "/__test/code");
    return response.ok ? response.text() : undefined;
  }, "test mailbox");
  await input('input[autocomplete="one-time-code"]', code);
  await waitText("Meta comments");
  const projectSlug = await evaluate(
    `fetch('/api/projects').then(r=>r.json()).then(b=>b.projects.find(p=>p.key === 'WP').slug)`,
  );
  if (!(await evaluate(`location.pathname === '/' + ${JSON.stringify(projectSlug)} + '/issues'`)))
    throw new Error("Sign-in did not open a project URL");
  checks++;
  if (projectSlug !== "wp-agent") throw new Error("Project URL must use a generated name slug");
  checks++;
  const oldProjectId = await evaluate(
    `fetch('/api/projects').then(r=>r.json()).then(b=>b.projects.find(p=>p.key === 'WP').id)`,
  );
  await navigate(`/${oldProjectId}/issues?view=board`, `/${projectSlug}/issues`);
  await until(
    () => evaluate(`location.search === '?view=board'`),
    "preserve filters on legacy ID redirect",
  );
  checks++;
  await navigate(`/${projectSlug}/issues`);
  await waitText("Meta comments");
  await click("New issue");
  if (!(await evaluate(`!document.querySelector('[role=dialog] select')`)))
    throw new Error("Issue form must not offer a project selector");
  checks++;
  await screenshot("new-issue-reference");
  await checkStatusMenu("Status", "new-issue-status-menu");
  await checkStatusMenu("Assignee", "new-issue-assignee-menu");
  await checkStatusMenu("Milestone", "new-issue-milestone-menu");
  await input('input[aria-label="Issue title"]', "Browser persisted issue");
  await evaluate(`document.querySelector('input[aria-label="Issue title"]').focus()`);
  await until(
    () =>
      evaluate(
        `(() => {const s = getComputedStyle(document.querySelector('input[aria-label="Issue title"]')); return s.outlineStyle === 'none' && s.borderBottomWidth === '1px' && s.borderBottomColor === 'rgb(100, 134, 255)';})()`,
      ),
    "focus inline input with a visible border",
  ).catch(async (error) => {
    console.error(
      await evaluate(
        `(() => {const e = document.querySelector('input[aria-label="Issue title"]'); const s = getComputedStyle(e); return {outline:s.outlineStyle, width:s.borderBottomWidth, color:s.borderBottomColor, focused:document.activeElement===e, className:e.className};})()`,
      ),
    );
    throw error;
  });
  checks++;

  await until(
    () =>
      evaluate(
        `!!document.querySelector('[contenteditable="true"][aria-label="Description or plan"]')`,
      ),
    "open description editor",
  );
  await evaluate(`(() => {
    const editor = document.querySelector('[contenteditable="true"][aria-label="Description or plan"]');
    editor.focus();
    const data = new DataTransfer();
    data.setData('text/plain', '## Goal\\nBrowser verification\\n- [ ] Browser checklist');
    editor.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
  })()`);
  await click("Labels");
  if (
    !(await evaluate(
      `document.querySelector('[aria-label="Choose labels"]')?.matches(':popover-open')`,
    ))
  )
    throw new Error("Label selector must escape modal clipping in the top layer");
  checks++;
  await screenshot("new-issue-label-menu");
  await input('input[aria-label="Search or create labels"]', "Browser label");
  await click('Create new label:"Browser label"');
  await waitText("Choose a color");
  if (!(await evaluate(`!document.querySelector('[aria-label="Remove Browser label label"]')`)))
    throw new Error("Label was attached before choosing a color");
  if (
    !(await evaluate(
      `fetch('/api/projects/WP/labels').then(r=>r.json()).then(b=>!b.labels.some(label=>label.name === 'Browser label'))`,
    ))
  )
    throw new Error("Label was saved before choosing a color");
  checks++;
  await click("Red");
  await until(
    () => evaluate(`!!document.querySelector('[aria-label="Remove Browser label label"]')`),
    "select the newly created label",
  );
  checks++;
  await evaluate(`document.querySelector('button[aria-label="Labels"]').click()`);
  await input('input[aria-label="Search or create labels"]', "browser LABEL");
  if (
    !(await evaluate(
      `!document.querySelector('[aria-label="Create new label"]') && [...document.querySelectorAll('[aria-label="Choose labels"] button')].some(button=>button.textContent.trim() === 'Browser label' && button.getAttribute('aria-pressed') === 'true')`,
    ))
  )
    throw new Error(
      "Searching for an existing label should show it without a duplicate creation action",
    );
  checks++;
  await evaluate(
    `document.querySelector('input[aria-label="Search or create labels"]').dispatchEvent(new KeyboardEvent('keydown', {key:'Enter', bubbles:true}))`,
  );
  await until(
    () => evaluate(`!document.querySelector('[aria-label="Remove Browser label label"]')`),
    "toggle matching label with Enter",
  );
  checks++;
  await input('input[aria-label="Search or create labels"]', "Browser label");
  await evaluate(
    `document.querySelector('input[aria-label="Search or create labels"]').dispatchEvent(new KeyboardEvent('keydown', {key:'Enter', bubbles:true}))`,
  );
  await until(
    () => evaluate(`!!document.querySelector('[aria-label="Remove Browser label label"]')`),
    "reselect the existing label",
  );
  checks++;
  await evaluate(
    `document.querySelector('input[aria-label="Search or create labels"]').dispatchEvent(new KeyboardEvent('keydown', {key:'Escape', bubbles:true}))`,
  );
  await call("Network.emulateNetworkConditions", {
    offline: false,
    latency: 200,
    downloadThroughput: 125000,
    uploadThroughput: 62500,
  });
  await evaluate(`(() => {
    const original = window.fetch.bind(window);
    const state = window.__issuePerf = {trace: [], blocked: [], failNext: true};
    window.fetch = async (path, options) => {
      if (String(path).includes('/issues') && options?.method === 'POST') {
        if (state.failNext) {
          state.failNext = false;
          return new Response(JSON.stringify({error: 'Simulated creation failure'}), {status: 503});
        }
        state.trace.push({event: 'POST started', time: performance.now()});
        const response = await original(path, options);
        state.trace.push({event: 'POST response', time: performance.now(), status: response.status});
        return response;
      }
      if (String(path).startsWith('/api/') && (!options?.method || options.method === 'GET')) {
        state.trace.push({event: 'GET held', path, time: performance.now()});
        return new Promise(resolve => state.blocked.push(() => resolve(original(path, options))));
      }
      return original(path, options);
    };
    state.restore = () => {window.fetch = original; state.blocked.forEach(release => release());};
  })()`);
  await click("Create issue");
  await waitText("Simulated creation failure");
  if (
    !(await evaluate(
      `document.querySelector('[role=dialog] input[aria-label="Issue title"]').value === 'Browser persisted issue'`,
    ))
  )
    throw new Error("Failed creation lost the issue draft");
  checks++;
  await click("Create issue");
  await until(
    () =>
      evaluate(
        `!document.querySelector('[role=dialog]') && document.body.innerText.includes('Browser persisted issue')`,
      ),
    "close the dialog and show the committed issue while all follow-up GETs are held",
  );
  console.log(
    "Issue creation network trace:",
    await evaluate(`(() => {
    const state = window.__issuePerf;
    state.trace.push({event: 'dialog closed and issue visible', time: performance.now()});
    state.restore();
    return state.trace;
  })()`),
  );
  checks++;
  await call("Network.emulateNetworkConditions", {
    offline: false,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1,
  });
  await waitText("Browser persisted issue");
  await evaluate(`document.querySelector('a[href="/${projectSlug}/issues/WP-2"]').click()`);
  await waitText("Browser checklist");
  await until(
    () =>
      evaluate(
        `Promise.all([fetch('/api/projects/WP/labels').then(r=>r.json()), fetch('/api/issues/WP-2').then(r=>r.json())]).then(([labels, detail])=>labels.labels.some(label=>label.name === 'Browser label' && label.color === '#f2a9ae' && detail.issue.label_ids.includes(label.id)))`,
      ),
    "persist label in the project and attach it to the issue",
  );
  checks++;

  if (
    !(await evaluate(
      `!!document.querySelector('section[aria-label="Subissues"]') && !!document.querySelector('aside section[aria-label="Subissues sidebar"]') && !document.querySelector('main aside').innerText.includes('Review rounds')`,
    ))
  )
    throw new Error("Subissues must appear below the plan and replace sidebar review rounds");
  checks++;
  await evaluate(
    `(() => { const trigger = document.querySelector('section[aria-label="Subissues"] button'); trigger.focus(); trigger.click(); })()`,
  );
  await checkStatusMenu("Subissue status", "new-subissue-status-menu");
  await input('input[aria-label="Subissue title"]', "Browser child");
  await until(
    () =>
      evaluate(
        `!!document.querySelector('[contenteditable="true"][aria-label="Subissue description"]')`,
      ),
    "open subissue Markdown editor",
  );
  await evaluate(`(() => {
    const editor = document.querySelector('[contenteditable="true"][aria-label="Subissue description"]');
    editor.focus();
    const data = new DataTransfer();
    data.setData('text/plain', '## Child task\\n\\n**Child description**');
    editor.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
  })()`);
  await screenshot("new-subissue-desktop");
  await click("Create subissue");
  await until(
    () =>
      evaluate(
        `!document.querySelector('[role=dialog]') && [...document.querySelectorAll('section[aria-label^="Subissues"]')].every(s=>s.innerText.includes('Browser child') && s.innerText.includes('WP-2.1'))`,
      ),
    "show the created subissue in both lists",
  );
  checks++;
  await evaluate(
    `document.querySelector('aside section[aria-label="Subissues sidebar"] li button').click()`,
  );
  await until(
    () => evaluate(`!!document.querySelector('[aria-label="Subissue description"] strong')`),
    "render saved subissue Markdown",
  );
  checks++;
  await evaluate(
    `document.querySelector('[role="combobox"][aria-label="Subissue status"]').click()`,
  );
  await evaluate(
    `[...document.querySelectorAll('[role="option"]')].find(e=>e.textContent.trim() === 'Done').click()`,
  );
  await click("Save subissue");
  await until(
    () =>
      evaluate(
        `!document.querySelector('[role=dialog]') && [...document.querySelectorAll('section[aria-label^="Subissues"]')].every(s=>!!s.querySelector('button[aria-label="WP-2.1: Browser child, Done"]') && s.innerText.includes('1/1'))`,
      ),
    "edit subissue status from the sidebar",
  );
  checks++;
  await navigate(`/${projectSlug}/issues/WP-2`);
  await waitText("Browser child");
  if (
    !(await evaluate(
      `fetch('/api/issues/WP-2/subissues').then(r=>r.json()).then(b=>b.subissues.length === 1 && b.subissues[0].description.includes('**Child description**') && b.subissues[0].status === 'done')`,
    ))
  )
    throw new Error("Subissue content did not persist");
  checks++;
  await evaluate(
    `fetch('/api/issues/WP-2/subissues', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({title:'Live child', status:'todo'})})`,
  );
  await until(
    () =>
      evaluate(
        `[...document.querySelectorAll('section[aria-label^="Subissues"]')].every(s=>s.innerText.includes('Live child'))`,
      ),
    "receive live subissue creation in both lists",
  );
  checks++;
  await screenshot("subissues-desktop");
  if (process.env.SPARROW_E2E_SCREENSHOT_DIR) {
    await call("Emulation.setDeviceMetricsOverride", {
      width: 390,
      height: 844,
      deviceScaleFactor: 1,
      mobile: true,
    });
    await screenshot("subissues-mobile");
    await evaluate(
      `(() => { const trigger = document.querySelector('section[aria-label="Subissues"] button'); trigger.focus(); trigger.click(); })()`,
    );
    await until(
      () => evaluate(`!!document.querySelector('[aria-label="Subissue title"]')`),
      "open mobile subissue dialog",
    );
    await screenshot("new-subissue-mobile");
    await checkStatusMenu("Subissue status", "new-subissue-mobile-status-menu");
    await evaluate(`document.querySelector('[role="dialog"] button[aria-label="Close"]').click()`);
    await until(
      () => evaluate(`!document.querySelector('[role=dialog]')`),
      "close mobile subissue dialog",
    );
    await call("Emulation.setDeviceMetricsOverride", {
      width: 1440,
      height: 1000,
      deviceScaleFactor: 1,
      mobile: false,
    });
  }

  await call("Emulation.setDeviceMetricsOverride", {
    width: 990,
    height: 542,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await evaluate(
    `(() => { const trigger = document.querySelector('section[aria-label="Subissues"] button'); trigger.focus(); trigger.click(); })()`,
  );
  await until(
    () => evaluate(`!!document.querySelector('[aria-label="Subissue title"]')`),
    "open subissue form at the reported viewport",
  );
  await checkStatusMenu("Subissue status", "subissue-status-reported-viewport");
  await input('input[aria-label="Subissue title"]', "Create more child");
  await evaluate(`document.querySelector('[role="dialog"] input[role="switch"]').click()`);
  await click("Create subissue");
  await until(
    () =>
      evaluate(
        `document.querySelector('[aria-label="Subissue title"]')?.value === '' && document.querySelector('[role="dialog"] button[type="submit"]')?.disabled === true`,
      ),
    "keep the form open and reset its draft after create more",
  );
  if (
    !(await evaluate(
      `fetch('/api/issues/WP-2/subissues').then(r=>r.json()).then(b=>b.subissues.some(child=>child.title === 'Create more child' && child.status === 'backlog'))`,
    ))
  )
    throw new Error("Create more did not persist the subissue with the selected status");
  checks++;
  if (
    !(await evaluate(`(() => {
    document.querySelector('[role="dialog"] button[aria-label="Close"]').click();
    return !!document.querySelector('[role="dialog"]');
  })()`))
  )
    throw new Error("Subissue dialog unmounted before its exit animation");
  await until(
    () => evaluate(`!!document.querySelector('.is-exiting [role="dialog"]')`),
    "animate the subissue dialog out",
  );
  checks++;
  await until(() => evaluate(`!document.querySelector('[role="dialog"]')`), "close subissue form");
  if (!(await evaluate(`document.activeElement?.getAttribute('aria-label') === 'Add subissue'`)))
    throw new Error("Closing the subissue dialog should restore focus to its trigger");
  checks++;
  await call("Emulation.clearDeviceMetricsOverride", {});

  await evaluate(`document.querySelector('input[type="checkbox"]').click()`);
  await until(
    () =>
      evaluate(
        `fetch('/api/issues/WP-2').then(r=>r.json()).then(b=>b.issue.checklist_progress.done === 1)`,
      ),
    "persist checklist",
  );
  checks++;
  await click("Comments");
  await until(
    () => evaluate(`!!document.querySelector('textarea[aria-label="Add a comment"]')`),
    "open the comments tab",
  );
  await input('textarea[aria-label="Add a comment"]', "Browser persisted comment");
  await click("Send");
  await waitText("Browser persisted comment");
  await navigate(`/${projectSlug}/issues/WP-2`);
  await click("Comments");
  await waitText("Browser persisted comment");
  if (
    !(await evaluate(
      `(() => {const chip=[...document.querySelectorAll('.label-chip')].find(e=>e.textContent === 'Browser label'); return chip && getComputedStyle(chip).color === 'rgb(242, 169, 174)';})()`,
    ))
  )
    throw new Error("Label color did not survive a reload");
  checks++;
  await click("Plan");
  await until(
    () => evaluate(`!!document.querySelector('[contenteditable="true"][aria-label="Issue plan"]')`),
    "open the plan tab",
  );
  await evaluate(`(() => {
    const editor = document.querySelector('[contenteditable="true"][aria-label="Issue plan"]');
    editor.focus();
    document.execCommand('selectAll');
    const data = new DataTransfer();
    data.setData('text/plain', '## Goal\\n\\nUpdated by browser with **live formatting**\\n\\n- [x] Browser checklist');
    editor.dispatchEvent(new ClipboardEvent('paste', {clipboardData: data, bubbles: true, cancelable: true}));
  })()`);
  await until(
    () =>
      evaluate(
        `fetch('/api/issues/WP-2').then(r=>r.json()).then(b=>b.issue.plan.includes('Updated by browser'))`,
      ),
    "automatically save the edited Markdown plan",
  );
  checks++;
  if (
    !(await evaluate(
      `!!document.querySelector('.issue-plan-editor h2') && !!document.querySelector('.issue-plan-editor strong') && ![...document.querySelectorAll('button')].some(b=>b.textContent === 'Edit plan' || b.textContent === 'Save plan')`,
    ))
  )
    throw new Error(
      "Markdown should render directly in the inline editor without edit or save buttons",
    );
  checks++;
  await navigate(`/${projectSlug}/issues/WP-2`);
  await waitText("Updated by browser with live formatting");
  await evaluate(`document.querySelector('[role="combobox"][aria-label="Priority"]').click()`);
  await evaluate(
    `[...document.querySelectorAll('[role="option"]')].find(e=>e.textContent.trim() === 'High').click()`,
  );
  await until(
    () =>
      evaluate(`fetch('/api/issues/WP-2').then(r=>r.json()).then(b=>b.issue.priority === 'high')`),
    "persist issue priority",
  );
  checks++;
  await evaluate(`document.querySelector('[role="combobox"][aria-label="Assignee"]').click()`);
  if (
    !(await evaluate(`fetch('/api/people').then(r=>r.json()).then(b=>{
    const options=[...document.querySelectorAll('[role="option"]')];
    const humans=b.actors.filter(a=>a.kind === 'human' && !a.disabled_at);
    return options.length === humans.length + 1 && humans.every(a=>options.some(o=>o.textContent.includes(a.display_name) && !!o.querySelector('.avatar')));
  })`))
  )
    throw new Error("Assignee options must show teammate avatars and exclude agents");
  checks++;
  await evaluate(`document.querySelectorAll('[role="option"]')[1].click()`);
  await until(
    () =>
      evaluate(`fetch('/api/issues/WP-2').then(r=>r.json()).then(b=>!!b.issue.assignee_actor_id)`),
    "assign a teammate",
  );
  checks++;
  await evaluate(`document.querySelector('button[aria-label="Milestone"]').click()`);
  await input('input[aria-label="Search or create milestone"]', "Browser milestone");
  await click("Create milestone: “Browser milestone”");
  await until(
    () =>
      evaluate(
        `Promise.all([fetch('/api/issues/WP-2').then(r=>r.json()),fetch('/api/projects/WP/milestones').then(r=>r.json())]).then(([detail,resources])=>resources.milestones.some(m=>m.name === 'Browser milestone' && m.id === detail.issue.milestone_id))`,
      ),
    "create and attach a milestone",
  );
  checks++;
  await evaluate(`document.querySelector('button[aria-label="Labels"]').click()`);
  await input('input[aria-label="Search or create labels"]', "Detail label");
  await click("Create labels: “Detail label”");
  await waitText("Choose a color");
  if (
    !(await evaluate(
      `fetch('/api/projects/WP/labels').then(r=>r.json()).then(b=>!b.labels.some(l=>l.name === 'Detail label'))`,
    ))
  )
    throw new Error("Detail label created before choosing its color");
  checks++;
  await click("Green");
  await until(
    () =>
      evaluate(
        `Promise.all([fetch('/api/issues/WP-2').then(r=>r.json()),fetch('/api/projects/WP/labels').then(r=>r.json())]).then(([detail,resources])=>resources.labels.some(l=>l.name === 'Detail label' && l.color === '#a1e4cd' && detail.issue.label_ids.includes(l.id)))`,
      ),
    "create a colored label on the issue page",
  );
  checks++;
  await navigate(`/${projectSlug}/issues/WP-2`);
  await waitText("Detail label");
  await waitText("Browser milestone");
  if (
    !(await evaluate(
      `document.querySelector('[role="combobox"][aria-label="Priority"]').textContent.includes('High')`,
    ))
  )
    throw new Error("Priority lost after reload");
  checks++;
  await evaluate(
    `fetch('/api/issues/WP-2', {method:'PATCH', headers:{'Content-Type':'application/json'}, body: JSON.stringify({title:'Live update from another client'})})`,
  );
  await until(
    () =>
      evaluate(
        `document.querySelector('input[aria-label="Issue title"]')?.value === "Live update from another client"`,
      ),
    "receive title updates",
  );
  checks++;
  await navigate(`/${projectSlug}/issues/WP-1/reviews/1`);
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

  await navigate(`/${projectSlug}/activity`);
  await waitText("Activity");
  await navigate("/settings");
  await waitText("Team members");
  await evaluate(`document.querySelector('button[aria-label="Edit profile"]').click()`);
  await waitText("Edit profile");
  await until(
    () =>
      evaluate(
        `fetch('/api/avatars').then(r=>r.json()).then(b=>document.querySelectorAll('[aria-label="Available avatars"] button').length === b.avatars.length + 1)`,
      ),
    "show every built-in avatar plus initials",
  );
  checks++;
  const avatarLoads = await evaluate(`(async () => {
    const url = new URL('/avatars/v1/nerdy-owl-astronomer-96.webp', location.href).href;
    performance.clearResourceTimings();
    const cold = await fetch(url, {cache: 'reload'});
    const bytes = (await cold.arrayBuffer()).byteLength;
    const warm = await fetch(url);
    await warm.arrayBuffer();
    return {bytes, cache: cold.headers.get('Cache-Control'), type: cold.headers.get('Content-Type'),
      loads: performance.getEntriesByName(url).map(entry => ({duration: entry.duration, transferred: entry.transferSize}))};
  })()`);
  if (
    avatarLoads.bytes > 20000 ||
    !avatarLoads.cache.includes("immutable") ||
    avatarLoads.type !== "image/webp"
  )
    throw new Error("Avatar must be compressed WebP with immutable caching");
  console.log("Cold and warm avatar loads:", avatarLoads);
  checks++;
  await input('[role="dialog"] input', "Cancelled profile name");
  await click("Cancel");
  await until(
    () => evaluate(`!document.querySelector('[role="dialog"]')`),
    "close profile without saving",
  );
  if (
    !(await evaluate(
      `fetch('/api/me').then(r=>r.json()).then(b=>b.actor.display_name !== 'Cancelled profile name')`,
    ))
  )
    throw new Error("Cancelling changed the profile");
  checks++;
  await evaluate(`document.querySelector('button[aria-label="Edit profile"]').click()`);
  await input('[role="dialog"] input', "Browser Profile");
  await evaluate(
    `document.querySelector('button[aria-label="Select adventurers cat pilot"]').click()`,
  );
  await click("Save changes");
  await until(
    () => evaluate(`!document.querySelector('[role="dialog"]')`),
    "save and close profile",
  );
  await navigate("/settings");
  await waitText("Browser Profile");
  if (
    !(await evaluate(
      `fetch('/api/me').then(r=>r.json()).then(b=>b.actor.avatar_id === 'adventurers-cat-pilot' && !!document.querySelector('img[src="/avatars/v1/adventurers-cat-pilot-48.webp"]'))`,
    ))
  )
    throw new Error("Profile avatar did not survive a reload");
  checks++;
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
  // All destructive setup below is restricted to the server's temporary test database.
  await evaluate(`fetch('/api/projects/WP', {method:'DELETE'})`);
  await navigate("/settings");
  await waitText("Create your first project");
  if (
    !(await evaluate(
      `!document.querySelector('aside, nav, header') && !document.querySelector('[aria-label="Navigation"]')`,
    ))
  )
    throw new Error("Onboarding must hide application navigation");
  checks++;
  await input('input[name="projectName"]', "First browser project");
  if (
    !(await evaluate(
      `(() => {const e = document.querySelector('input[name="projectName"]'); const s = getComputedStyle(e); return s.outlineStyle === 'none' && s.transitionProperty === 'border-color';})()`,
    ))
  )
    throw new Error("Shared input focus must use a border transition without an outline");
  checks++;
  if (!(await evaluate(`document.querySelector('input[name="projectKey"]').value === 'FBP'`)))
    throw new Error("Project key was not generated from the name");
  checks++;
  await input('input[name="projectKey"]', "FIRST");
  await input('input[name="projectName"]', "First browser project renamed");
  if (!(await evaluate(`document.querySelector('input[name="projectKey"]').value === 'FIRST'`)))
    throw new Error("Typing the name overwrote the edited project key");
  checks++;
  await click("Create project");
  await waitText("New issue");
  const firstSlug = await evaluate(
    `fetch('/api/projects').then(r=>r.json()).then(b=>b.projects.find(p=>p.key === 'FIRST').slug)`,
  );
  if (
    !(await evaluate(
      `location.pathname === '/' + ${JSON.stringify(firstSlug)} + '/issues' && !!document.querySelector('aside')`,
    ))
  )
    throw new Error("New project must open with navigation");
  checks++;
  await click("New issue");
  await input('input[aria-label="Issue title"]', "Only in first project");
  await click("Create issue");
  await waitText("Only in first project");
  await until(() => evaluate(`!document.querySelector('[role=dialog]')`), "close issue dialog");
  await evaluate(
    `(() => {const button = document.querySelector('button[aria-label="Add project"]'); button.focus(); button.click();})()`,
  );
  await waitText("New project");
  await evaluate(
    `document.querySelector('[role=dialog]').dispatchEvent(new KeyboardEvent('keydown', {key:'Escape', bubbles:true}))`,
  );
  await until(
    () =>
      evaluate(
        `!document.querySelector('[role=dialog]') && document.activeElement?.getAttribute('aria-label') === 'Add project'`,
      ),
    "dismiss project modal and restore focus",
  );
  checks++;
  await evaluate(
    `(() => {const button = document.querySelector('button[aria-label="Add project"]'); button.focus(); button.click();})()`,
  );
  await waitText("New project");
  await input('input[name="projectName"]', "Second browser project");
  await input('input[name="projectKey"]', "SECOND");
  await click("Create project");
  const secondSlug = await until(
    () =>
      evaluate(
        `fetch('/api/projects').then(r=>r.json()).then(b=>b.projects.find(p=>p.key === 'SECOND')?.slug)`,
      ),
    "create second project",
  );
  await until(
    () => evaluate(`location.pathname === '/' + ${JSON.stringify(secondSlug)} + '/issues'`),
    "open second project",
  );
  await waitText("New issue");
  await waitText("Second browser project / Issues");
  await until(
    () => evaluate(`!document.querySelector('[role=dialog]')`),
    "close project modal after creation",
  );
  if (!(await evaluate(`!document.querySelector('main').innerText.includes('FIRST-1')`)))
    throw new Error("Issue list leaked another project");
  checks++;
  await click("New issue");
  await input('input[aria-label="Issue title"]', "Only in second project");
  await click("Create issue");
  await waitText("Only in second project");
  await until(
    () => evaluate(`!document.querySelector('[role=dialog]')`),
    "close second issue dialog",
  );
  if (
    !(await evaluate(
      `fetch('/api/projects/SECOND/issues').then(r=>r.json()).then(b=>b.issues.some(i=>i.key === 'SECOND-1' && i.title === 'Only in second project'))`,
    ))
  )
    throw new Error("Issue was not created in the URL's project");
  checks++;
  await navigate(`/${secondSlug}/issues/FIRST-1`);
  await waitText("Issue not found");
  await navigate(`/${secondSlug}/issues/FIRST-1/reviews/1`);
  await waitText("Issue not found");
  await navigate("/missing-project/issues");
  await waitText("Project not found");
  await navigate(`/${secondSlug}/milestones`);
  await waitText("Track the work that matters for your project.");
  await navigate(`/${secondSlug}/findings`);
  await waitText("No findings found");
  await navigate(`/${secondSlug}/activity`);
  await waitText("Activity");
  if (!(await evaluate(`!document.querySelector('main').innerText.includes('FIRST-1')`)))
    throw new Error("Activity leaked another project");
  checks++;
  await navigate("/activity");
  await waitText("Track what’s happening across your projects, issues, and agents.");
  await navigate("/issues");
  await waitText("Assigned to you");
  if (
    !(await evaluate(
      `(() => {const nav = document.querySelector('nav[aria-label="Main navigation"]'); const groups = nav.querySelectorAll('section'); return nav.querySelector('a[href="/issues"]').textContent.includes('My issues') && nav.querySelector('a[href="/activity"]').textContent.includes('Activity') && groups.length === 2 && [...groups].every(g => g.querySelectorAll('a').length === 3 && g.textContent.includes('Issues') && g.textContent.includes('Milestones') && g.textContent.includes('Open findings'));})()`,
    ))
  )
    throw new Error("Sidebar must keep root links and three nested links per project");
  checks++;
  await navigate("/settings/sessions");
  await waitText("Current");
  await click("Sign out all devices");
  await waitText("Sign in");
  if (browserErrors.length) throw new Error(`${browserErrors.length} browser runtime errors`);
  console.log(`${checks} browser assertions passed`);
} finally {
  socket?.close();
  browser.kill();
  await browser.exited;
  await rm(profile, { recursive: true, force: true });
}
