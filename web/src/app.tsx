import { Columns3 } from "lucide-react";
import { Outlet, useRouter, useRouterState } from "@tanstack/react-router";
import { filterIssues } from "./lib/issue-filter";
import { Select } from "./components/select";
import { Icon, type IconName } from "./components/icon";
import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type FormEvent,
} from "react";

type Page =
  | "issues"
  | "issue"
  | "review"
  | "findings"
  | "milestones"
  | "activity"
  | "settings"
  | "sessions"
  | "signin";
type IssueStatus = "In Review" | "In Progress" | "Todo" | "Backlog" | "Done" | "Canceled";
type Issue = {
  key: string;
  title: string;
  status: IssueStatus;
  labels: string[];
  assignee: string;
  findings: number;
  milestone: string;
  updated: string;
};
type Finding = {
  id: string;
  issue: string;
  round: number;
  title: string;
  severity: "Blocking" | "Should fix" | "Nit";
  author: string;
  status: string;
  problem: string;
  file: string;
};
const initialIssues: Issue[] = [
  {
    key: "SP-142",
    title: "Add email code sign-in",
    status: "In Review",
    labels: ["auth", "frontend", "security"],
    assignee: "Shanto",
    findings: 3,
    milestone: "Launch v1.0",
    updated: "Nov 6, 2024",
  },
  {
    key: "SP-144",
    title: "Clarify error message for invalid login",
    status: "In Review",
    labels: ["UX", "frontend"],
    assignee: "Claude Code",
    findings: 1,
    milestone: "Launch v1.0",
    updated: "Nov 5, 2024",
  },
  {
    key: "SP-148",
    title: "Add telemetry for sign-in failures",
    status: "In Review",
    labels: ["Observability"],
    assignee: "Shanto",
    findings: 2,
    milestone: "Launch v1.0",
    updated: "Nov 4, 2024",
  },
  {
    key: "SP-136",
    title: "Validate branch names",
    status: "In Progress",
    labels: ["backend", "security"],
    assignee: "Claude Code",
    findings: 2,
    milestone: "Launch v1.0",
    updated: "Nov 6, 2024",
  },
  {
    key: "SP-139",
    title: "Improve empty state copy",
    status: "In Progress",
    labels: ["UX"],
    assignee: "Codex",
    findings: 1,
    milestone: "Launch v1.0",
    updated: "Nov 6, 2024",
  },
  {
    key: "SP-141",
    title: "Add keyboard shortcuts",
    status: "In Progress",
    labels: ["frontend"],
    assignee: "Priya",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 5, 2024",
  },
  {
    key: "SP-140",
    title: "Add branch creation flow",
    status: "Todo",
    labels: ["feature"],
    assignee: "Shanto",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 4, 2024",
  },
  {
    key: "SP-146",
    title: "Improve search performance",
    status: "Todo",
    labels: ["performance"],
    assignee: "Claude Code",
    findings: 2,
    milestone: "Launch v1.0",
    updated: "Nov 4, 2024",
  },
  {
    key: "SP-150",
    title: "Dark mode polish",
    status: "Todo",
    labels: ["UI"],
    assignee: "Priya",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 3, 2024",
  },
  {
    key: "SP-152",
    title: "Export findings to CSV",
    status: "Backlog",
    labels: ["feature"],
    assignee: "Codex",
    findings: 0,
    milestone: "Later",
    updated: "Oct 31, 2024",
  },
  {
    key: "SP-153",
    title: "Add issue filters to the project list",
    status: "In Review",
    labels: ["frontend"],
    assignee: "Priya",
    findings: 1,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-154",
    title: "Handle concurrent review decisions",
    status: "In Review",
    labels: ["bug"],
    assignee: "Codex",
    findings: 2,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-155",
    title: "Stream project activity updates",
    status: "In Progress",
    labels: ["realtime"],
    assignee: "Claude Code",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-156",
    title: "Improve review finding keyboard actions",
    status: "In Progress",
    labels: ["accessibility"],
    assignee: "Shanto",
    findings: 1,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-157",
    title: "Add project milestone settings",
    status: "Todo",
    labels: ["feature"],
    assignee: "Codex",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-158",
    title: "Document agent token rotation",
    status: "Todo",
    labels: ["documentation"],
    assignee: "Priya",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-159",
    title: "Add finding discussion threads",
    status: "Backlog",
    labels: ["frontend"],
    assignee: "Claude Code",
    findings: 0,
    milestone: "Later",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-160",
    title: "Support duplicate finding links",
    status: "Backlog",
    labels: ["backend"],
    assignee: "Shanto",
    findings: 0,
    milestone: "Later",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-161",
    title: "Add email code expiration checks",
    status: "Done",
    labels: ["auth"],
    assignee: "Codex",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-162",
    title: "Create project issue counters",
    status: "Done",
    labels: ["database"],
    assignee: "Claude Code",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-163",
    title: "Add issue status transitions",
    status: "Done",
    labels: ["backend"],
    assignee: "Priya",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-164",
    title: "Explore alternate sign-in flow",
    status: "Canceled",
    labels: ["research"],
    assignee: "Shanto",
    findings: 0,
    milestone: "Later",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-165",
    title: "Review backup failure messaging",
    status: "Todo",
    labels: ["security"],
    assignee: "Codex",
    findings: 0,
    milestone: "Later",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-166",
    title: "Add responsive layout for review cards",
    status: "In Progress",
    labels: ["UI"],
    assignee: "Priya",
    findings: 1,
    milestone: "Launch v1.0",
    updated: "Nov 7, 2024",
  },
  {
    key: "SP-167",
    title: "Add session device management",
    status: "Done",
    labels: ["security"],
    assignee: "Shanto",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 8, 2024",
  },
  {
    key: "SP-168",
    title: "Add review round commit ranges",
    status: "Done",
    labels: ["backend"],
    assignee: "Claude Code",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 8, 2024",
  },
  {
    key: "SP-169",
    title: "Explore alternative review layouts",
    status: "Canceled",
    labels: ["research"],
    assignee: "Priya",
    findings: 0,
    milestone: "Later",
    updated: "Nov 8, 2024",
  },
  {
    key: "SP-170",
    title: "Investigate external notification support",
    status: "Canceled",
    labels: ["research"],
    assignee: "Codex",
    findings: 0,
    milestone: "Later",
    updated: "Nov 8, 2024",
  },
  {
    key: "SP-171",
    title: "Add finding search across review rounds",
    status: "Backlog",
    labels: ["feature"],
    assignee: "Shanto",
    findings: 0,
    milestone: "Later",
    updated: "Nov 8, 2024",
  },
  {
    key: "SP-172",
    title: "Improve empty milestone states",
    status: "Todo",
    labels: ["UX"],
    assignee: "Priya",
    findings: 0,
    milestone: "Launch v1.0",
    updated: "Nov 8, 2024",
  },
];
const initialFindings: Finding[] = [
  {
    id: "R1",
    issue: "SP-142",
    round: 2,
    title: "Session can expire during sign-in",
    severity: "Blocking",
    author: "Codex",
    status: "Open",
    problem:
      "The sign-in flow can take longer than expected, causing the session to expire before the user completes authentication. This results in an error and the user having to start over.",
    file: "server/internal/auth/session.go:42",
  },
  {
    id: "R2",
    issue: "SP-142",
    round: 2,
    title: "Missing validation for branch name",
    severity: "Should fix",
    author: "Claude Code",
    status: "Open",
    problem: "Branch names are not validated, allowing invalid characters that fail on push.",
    file: "server/internal/store/issue.go:68",
  },
  {
    id: "R3",
    issue: "SP-142",
    round: 2,
    title: "Clarify empty state copy",
    severity: "Nit",
    author: "Codex",
    status: "Open",
    problem:
      "The empty state copy is a bit vague and could better explain next steps for new users.",
    file: "web/src/routes/index.tsx:24",
  },
  {
    id: "R1",
    issue: "SP-144",
    round: 1,
    title: "Handle expired sign-in codes",
    severity: "Should fix",
    author: "Claude Code",
    status: "Open",
    problem: "The error does not explain how to request a new code.",
    file: "web/src/routes/sign-in.tsx:32",
  },
];
const statuses: IssueStatus[] = ["In Review", "In Progress", "Todo", "Backlog", "Done", "Canceled"];
const statusIcon: Record<IssueStatus, IconName> = {
  "In Review": "circle-dot",
  "In Progress": "circle-dot",
  Todo: "circle",
  Backlog: "circle-dashed",
  Done: "circle-check",
  Canceled: "circle-x",
};

type AppContextValue = {
  issues: Issue[];
  findings: Finding[];
  currentIssue: Issue;
  setIssues: React.Dispatch<React.SetStateAction<Issue[]>>;
  go: (page: Page, issue?: string, round?: string) => void;
  decide: (finding: Finding, status: string) => void;
  notify: (message: string) => void;
  openNew: () => void;
};
const AppContext = createContext<AppContextValue | null>(null);

/** Renders the content belonging to an individual route. */
export function PageView({ page }: { page: Page }) {
  const context = useContext(AppContext);
  if (!context) throw new Error("PageView requires AppContext");
  const { issues, findings, currentIssue, setIssues, go, decide, notify, openNew } = context;
  if (page === "issues")
    return <Issues issues={issues} open={(key) => go("issue", key)} onNew={openNew} />;
  if (page === "issue")
    return (
      <IssuePage
        issue={currentIssue}
        go={go}
        availableLabels={[
          ...new Set([...initialIssues, ...issues].flatMap((item) => item.labels)),
        ].sort()}
        onLabels={(labels) =>
          setIssues((items) =>
            items.map((item) => (item.key === currentIssue.key ? { ...item, labels } : item)),
          )
        }
        onStatus={(status) =>
          setIssues((items) =>
            items.map((item) => (item.key === currentIssue.key ? { ...item, status } : item)),
          )
        }
      />
    );
  if (page === "review")
    return (
      <Review
        issue={currentIssue}
        findings={findings.filter((item) => item.issue === currentIssue.key)}
        go={go}
        decide={decide}
      />
    );
  if (page === "findings") return <Findings findings={findings} go={go} decide={decide} />;
  if (page === "activity") return <Activity go={go} />;
  if (page === "milestones") return <Milestones go={go} />;
  if (page === "settings" || page === "sessions")
    return <Settings page={page} go={go} notify={notify} />;
  return <SignIn onContinue={() => go("issues")} />;
}

export function App() {
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const router = useRouter();
  const parts = pathname.split("/").filter(Boolean);
  const staticPages: Record<string, Page> = {
    issues: "issues",
    findings: "findings",
    milestones: "milestones",
    activity: "activity",
    settings: "settings",
    signin: "signin",
  };
  const page: Page =
    parts[0] === "settings" && parts[1] === "sessions"
      ? "sessions"
      : (staticPages[parts[0] ?? ""] ??
        (parts[1] === "reviews" ? "review" : parts[0] ? "issue" : "issues"));
  const selectedIssue = page === "issue" || page === "review" ? parts[0] : undefined;
  const [issues, setIssues] = useState(initialIssues);
  const [findings, setFindings] = useState(initialFindings);
  const [palette, setPalette] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [showNew, setShowNew] = useState(false);
  const [title, setTitle] = useState("");
  const [newLabels, setNewLabels] = useState<string[]>([]);
  const [toast, setToast] = useState("");
  const go = (next: Page, issue?: string, round = "2") => {
    if (next === "issue")
      void router.navigate({ to: "/$issue", params: { issue: issue ?? "SP-142" } });
    else if (next === "review")
      void router.navigate({
        to: "/$issue/reviews/$round",
        params: { issue: issue ?? "SP-142", round },
      });
    else {
      const paths = {
        issues: "/issues",
        findings: "/findings",
        milestones: "/milestones",
        activity: "/activity",
        settings: "/settings",
        sessions: "/settings/sessions",
        signin: "/signin",
      } as const;
      void router.navigate({ to: paths[next] });
    }
    setPalette(false);
  };
  const currentIssue = issues.find((item) => item.key === selectedIssue) ?? issues[0]!;
  const notify = (message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(""), 3000);
  };
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette(true);
      }
      if (e.key === "Escape") {
        setPalette(false);
        setShowNew(false);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  const addIssue = (e: FormEvent) => {
    e.preventDefault();
    const key = `SP-${Math.max(...issues.map((item) => Number(item.key.split("-")[1]))) + 1}`;
    setIssues((current) => [
      {
        key,
        title: title.trim(),
        status: "Backlog",
        labels: newLabels,
        assignee: "Shanto",
        findings: 0,
        milestone: "Later",
        updated: "Just now",
      },
      ...current,
    ]);
    setTitle("");
    setNewLabels([]);
    setShowNew(false);
    notify(`${key} created in this preview`);
  };
  const decide = (finding: Finding, status: string) => {
    setFindings((current) =>
      current.map((item) =>
        item.issue === finding.issue && item.round === finding.round && item.id === finding.id
          ? { ...item, status }
          : item,
      ),
    );
    notify(`${finding.id} marked ${status.toLowerCase()} in this preview`);
  };
  const context = {
    issues,
    findings,
    currentIssue,
    setIssues,
    go,
    decide,
    notify,
    openNew: () => {
      setNewLabels([]);
      setShowNew(true);
    },
  };
  if (page === "signin")
    return (
      <AppContext.Provider value={context}>
        <Outlet />
      </AppContext.Provider>
    );

  return (
    <AppContext.Provider value={context}>
      <div className="app-shell">
        <aside className="sidebar">
          <div className="brand">
            <Icon name="bird" className="brand-mark" /> Super Sparrow
          </div>
          <nav className="sidebar-nav" aria-label="Main navigation">
            <Nav
              icon="circle-user-round"
              label="My issues"
              active={false}
              onClick={() => go("issues")}
            />
            <Nav
              icon="activity"
              label="Activity"
              active={page === "activity"}
              onClick={() => go("activity")}
            />
            <div className="nav-heading">
              Projects{" "}
              <button
                className="nav-add"
                aria-label="Add project"
                onClick={() => notify("Project creation is a preview")}
              >
                <Icon name="plus" />
              </button>
            </div>
            <button className="project-button" onClick={() => setCollapsed(!collapsed)}>
              <Icon name="folder" className="project-logo" /> <strong>Sparrow</strong>
              <Icon name={collapsed ? "chevron-right" : "chevron-down"} className="chevron" />
            </button>
            {!collapsed && (
              <div className="project-nav">
                <Nav
                  icon="list-todo"
                  label="Issues"
                  active={["issues", "issue", "review"].includes(page)}
                  onClick={() => go("issues")}
                />
                <Nav
                  icon="target"
                  label="Milestones"
                  active={page === "milestones"}
                  onClick={() => go("milestones")}
                />
                <Nav
                  icon="list-filter"
                  label="Open findings"
                  active={page === "findings"}
                  onClick={() => go("findings")}
                />
              </div>
            )}
          </nav>
          <div className="sidebar-bottom">
            <button className="text-button signout-preview" onClick={() => go("signin")}>
              Sign-in preview
            </button>
            <Nav
              icon="settings"
              label="Settings"
              active={page === "settings" || page === "sessions"}
              onClick={() => go("settings")}
            />
          </div>
        </aside>
        <main className="main-area">
          <Outlet />
        </main>
        {toast && (
          <div className="toast" role="status">
            {toast}
          </div>
        )}
        {palette && (
          <div className="overlay" onMouseDown={() => setPalette(false)}>
            <div
              className="palette"
              role="dialog"
              aria-modal="true"
              aria-label="Navigation"
              onMouseDown={(e) => e.stopPropagation()}
            >
              <div className="palette-title">
                Go to <kbd>esc</kbd>
              </div>
              {(
                [
                  ["issues", "Issues"],
                  ["findings", "Open findings"],
                  ["milestones", "Milestones"],
                  ["activity", "Activity"],
                  ["settings", "People and agents"],
                ] as [Page, string][]
              ).map(([key, label]) => (
                <button key={key} onClick={() => go(key)}>
                  {label}
                  <span>↗</span>
                </button>
              ))}
            </div>
          </div>
        )}
        {showNew && (
          <div className="overlay">
            <div className="modal" role="dialog" aria-modal="true" aria-label="New issue">
              <form onSubmit={addIssue}>
                <h2>Create issue</h2>
                <label>
                  Title
                  <input
                    autoFocus
                    required
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="Issue title"
                  />
                </label>
                <LabelPicker
                  options={[
                    ...new Set([...initialIssues, ...issues].flatMap((item) => item.labels)),
                  ].sort()}
                  selected={newLabels}
                  onChange={setNewLabels}
                />
                <div className="modal-actions">
                  <button className="button" type="button" onClick={() => setShowNew(false)}>
                    Cancel
                  </button>
                  <button className="primary" disabled={!title.trim()}>
                    Create issue
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </AppContext.Provider>
  );
}
function SignIn({ onContinue }: { onContinue: () => void }) {
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);
  return (
    <div className="signin-page">
      <div className="signin-brand">
        <Icon name="bird" className="brand-mark" /> Super Sparrow
      </div>
      <form
        className="signin-card"
        onSubmit={(event) => {
          event.preventDefault();
          if (sent) onContinue();
          else setSent(true);
        }}
      >
        <Icon name="bird" className="signin-icon" />
        <h1>{sent ? "Check your email" : "Welcome to Super Sparrow"}</h1>
        <p className="muted">
          {sent
            ? "If this email belongs to a teammate, a sign-in code is on its way."
            : "Sign in with your team email to continue."}
        </p>
        <label>
          Email
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            disabled={sent}
            placeholder="you@example.com"
          />
        </label>
        {sent && (
          <label>
            Sign-in code
            <input
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              required
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="000000"
            />
          </label>
        )}
        <button className="primary full">{sent ? "Sign in" : "Continue with email"}</button>
        {sent && (
          <button
            type="button"
            className="text-button centered"
            onClick={() => {
              setSent(false);
              setCode("");
            }}
          >
            Use another email
          </button>
        )}
        <p className="muted small">Frontend preview only. No email is sent.</p>
      </form>
    </div>
  );
}
function Nav({
  icon,
  label,
  active,
  onClick,
}: {
  icon: IconName;
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button className={`nav-item ${active ? "active" : ""}`} onClick={onClick}>
      <Icon name={icon} className="nav-icon" />
      {label}
    </button>
  );
}
function Avatar({ name }: { name: string }) {
  if (name === "Codex" || name === "Claude Code")
    return (
      <img
        className="agent-icon"
        src={`/brands/${name === "Codex" ? "codex" : "claude"}.svg`}
        alt=""
      />
    );
  return <span className="avatar">{name.slice(0, 2).toUpperCase()}</span>;
}
function Person({ name }: { name: string }) {
  return (
    <span className="person-inline">
      <Avatar name={name} />
      {name}
    </span>
  );
}
function Severity({ value }: { value: Finding["severity"] }) {
  return (
    <span className={`severity severity-${value.toLowerCase().replace(" ", "-")}`}>{value}</span>
  );
}
function Header({
  crumb,
  title,
  description,
  action,
}: {
  crumb: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <header className="view-header">
      <div className="breadcrumb">{crumb}</div>
      <div className="title-row">
        <div>
          <h1>{title}</h1>
          {description && <p className="muted header-description">{description}</p>}
        </div>
        {action}
      </div>
    </header>
  );
}
function Issues({
  issues,
  open,
  onNew,
}: {
  issues: Issue[];
  open: (key: string) => void;
  onNew: () => void;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("All");
  const [assignee, setAssignee] = useState("All");
  const [label, setLabel] = useState("All");
  const [milestone, setMilestone] = useState("All");
  const [closed, setClosed] = useState<string[]>([]);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [toolbarHeight, setToolbarHeight] = useState(0);
  useEffect(() => {
    const toolbar = toolbarRef.current;
    if (!toolbar) return;

    const observer = new ResizeObserver(() =>
      setToolbarHeight(toolbar.getBoundingClientRect().height),
    );
    observer.observe(toolbar);
    return () => observer.disconnect();
  }, []);
  const router = useRouter();
  const { pathname, search } = useRouterState({ select: (state) => state.location });
  const view = search.view === "board" ? "board" : "list";
  const setView = (next: "list" | "board") => {
    void router.navigate({
      to: pathname === "/" ? "/" : "/issues",
      search: { view: next === "board" ? "board" : undefined },
    });
  };
  const filtered = filterIssues(issues, { query, status, assignee, label, milestone });
  return (
    <>
      <div className="issues-toolbar" ref={toolbarRef}>
        <div className="issue-controls">
          <div className="issue-topline">
            <div className="breadcrumb">Projects / Sparrow</div>
          </div>
          <div className="filters issue-filters">
            <input
              className="filter-search"
              placeholder="Search issues..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              aria-label="Search issues"
            />
            <Filter name="Status" value={status} onChange={setStatus} options={statuses} />
            <Filter
              name="Assignee"
              value={assignee}
              onChange={setAssignee}
              options={[...new Set(issues.map((item) => item.assignee))]}
            />
            <Filter
              name="Label"
              value={label}
              onChange={setLabel}
              options={[
                ...new Set([...initialIssues, ...issues].flatMap((item) => item.labels)),
              ].sort()}
            />
            <Filter
              name="Milestone"
              value={milestone}
              onChange={setMilestone}
              options={[...new Set(issues.map((item) => item.milestone))]}
            />
            <div className="view-switch" role="group" aria-label="Issue view">
              <button type="button" aria-pressed={view === "list"} onClick={() => setView("list")}>
                <Icon name="list" /> List
              </button>
              <button
                type="button"
                aria-pressed={view === "board"}
                onClick={() => setView("board")}
              >
                <Columns3 aria-hidden="true" className="icon" /> Board
              </button>
            </div>
            <button className="primary new-issue-button" onClick={onNew}>
              <Icon name="plus" /> &nbsp; New issue
            </button>
          </div>
        </div>
      </div>
      {view === "board" ? (
        <div className="issue-board app-scrollbar" aria-label="Issues by status">
          {statuses
            .filter((group) => status === "All" || group === status)
            .map((group) => {
              const rows = filtered.filter((issue) => issue.status === group);
              return (
                <section className="board-column" key={group} aria-label={group}>
                  <div className="board-heading">
                    <span className={`status-icon status-${group.toLowerCase().replace(" ", "-")}`}>
                      <Icon name={statusIcon[group]} />
                    </span>
                    <strong>{group}</strong>
                    <span className="count">{rows.length}</span>
                  </div>
                  <div
                    className="board-cards app-scrollbar"
                    role="region"
                    aria-label={`${group} issues`}
                    tabIndex={0}
                  >
                    {rows.map((issue) => (
                      <button
                        className="board-card"
                        key={issue.key}
                        onClick={() => open(issue.key)}
                      >
                        <span className="board-card-key">{issue.key}</span>
                        <strong>{issue.title}</strong>
                        <LabelChips labels={issue.labels} />
                        <span className="board-card-footer">
                          <Person name={issue.assignee} />
                          <span className="muted">
                            <Icon name="message-square" /> &nbsp;{issue.findings}
                          </span>
                        </span>
                      </button>
                    ))}
                    {rows.length === 0 && <p className="board-empty">No issues</p>}
                  </div>
                </section>
              );
            })}
        </div>
      ) : (
        <div
          className="list-content app-scrollbar"
          style={{ "--issues-toolbar-height": `${toolbarHeight}px` } as CSSProperties}
        >
          {statuses.map((group) => {
            const rows = filtered.filter((item) => item.status === group);
            if (!rows.length) return null;
            return (
              <section key={group}>
                <button
                  className={`group-heading group-heading--${group.toLowerCase().replace(" ", "-")}`}
                  aria-expanded={!closed.includes(group)}
                  onClick={() =>
                    setClosed((items) =>
                      items.includes(group) ? items.filter((x) => x !== group) : [...items, group],
                    )
                  }
                >
                  <Icon name={closed.includes(group) ? "chevron-right" : "chevron-down"} />
                  <span className={`status-icon status-${group.toLowerCase().replace(" ", "-")}`}>
                    <Icon name={statusIcon[group]} />
                  </span>
                  <strong>{group}</strong>
                  <span className="count">{rows.length}</span>
                </button>
                {!closed.includes(group) &&
                  rows.map((item) => (
                    <button
                      className="issue-grid issue-row"
                      key={item.key}
                      onClick={() => open(item.key)}
                    >
                      <span className="issue-key">{item.key}</span>
                      <span className="issue-title">{item.title}</span>
                      <LabelChips labels={item.labels} />
                      <span
                        className="issue-assignee"
                        role="img"
                        aria-label={`Assigned to ${item.assignee}`}
                        title={item.assignee}
                      >
                        <Avatar name={item.assignee} />
                      </span>
                    </button>
                  ))}
              </section>
            );
          })}
          {!filtered.length && (
            <div className="empty-state">
              <h2>No issues found</h2>
              <p>Try changing your filters.</p>
            </div>
          )}
        </div>
      )}
    </>
  );
}
function LabelChips({ labels }: { labels: string[] }) {
  return (
    <span className="issue-labels">
      {labels.map((label) => (
        <span className={`label-chip label-${label.toLowerCase()}`} key={label}>
          {label}
        </span>
      ))}
    </span>
  );
}

function LabelPicker({
  options,
  selected,
  onChange,
}: {
  options: string[];
  selected: string[];
  onChange: (labels: string[]) => void;
}) {
  return (
    <fieldset className="label-picker">
      <legend>Labels</legend>
      <div className="label-picker-options">
        {options.map((label) => (
          <button
            type="button"
            key={label}
            className={`label-picker-option ${selected.includes(label) ? "is-selected" : ""}`}
            aria-pressed={selected.includes(label)}
            onClick={() =>
              onChange(
                selected.includes(label)
                  ? selected.filter((item) => item !== label)
                  : [...selected, label],
              )
            }
          >
            {label}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

function Filter({
  name,
  value,
  onChange,
  options,
}: {
  name: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
}) {
  return (
    <Select
      variant="filter"
      label={name}
      value={value}
      onChange={onChange}
      options={["All", ...options].map((item) => ({ value: item, label: item }))}
    />
  );
}
function IssuePage({
  issue,
  go,
  onStatus,
  availableLabels,
  onLabels,
}: {
  issue: Issue;
  go: (page: Page, issue?: string, round?: string) => void;
  onStatus: (status: IssueStatus) => void;
  availableLabels: string[];
  onLabels: (labels: string[]) => void;
}) {
  const [editingLabels, setEditingLabels] = useState(false);
  const [tab, setTab] = useState("Plan");
  const [checks, setChecks] = useState([true, true, false, false, false]);
  return (
    <div className="issue-layout">
      <div className="issue-body">
        <div className="breadcrumb">
          <button onClick={() => go("issues")}>Issues</button> / {issue.key}
        </div>
        <h1>{issue.title}</h1>
        <div className="issue-meta">
          <Icon name="circle-dot" /> &nbsp;{issue.key}　 ·　 <Icon name="git-branch" /> &nbsp;
          feat/sp-142-email-code　 ·　 <Icon name="message-square" /> &nbsp; 8 comments
        </div>
        <p className="issue-summary">
          Adds sign-in with email code, branch creation flow, and related UI updates.
        </p>
        <div className="tabs issue-tabs">
          {["Plan", "Review rounds", "Activity"].map((name) => (
            <button
              key={name}
              className={tab === name ? "selected" : ""}
              onClick={() => setTab(name)}
            >
              {name} {name === "Review rounds" && <span className="count">2</span>}
            </button>
          ))}
        </div>
        {tab === "Plan" ? (
          <div className="plan">
            <section>
              <Icon name="target" className="plan-icon" />
              <div>
                <h2>Goal</h2>
                <p>
                  Allow users to sign in with a one-time code sent to their email address. Provide a
                  seamless, secure flow with clear messaging and handle edge cases gracefully.
                </p>
              </div>
            </section>
            <section>
              <Icon name="list" className="plan-icon" />
              <div>
                <h2>Approach</h2>
                <ol>
                  <li>Add backend endpoint to request and verify email codes.</li>
                  <li>Implement UI for requesting a code and entering the code.</li>
                  <li>Integrate the authentication flow with session management.</li>
                  <li>Update related loading, error, and success states.</li>
                </ol>
              </div>
            </section>
            <section>
              <Icon name="triangle-alert" className="plan-icon" />
              <div>
                <h2>Edge cases</h2>
                <ul>
                  <li>Expired code — show a clear error and allow requesting a new code.</li>
                  <li>Too many requests — rate limit with a helpful message.</li>
                  <li>Network errors — offer a retry option.</li>
                </ul>
              </div>
            </section>
            <section>
              <Icon name="square-check" className="plan-icon" />
              <div>
                <h2>Checklist</h2>
                {[
                  "Backend endpoints (request + verify code)",
                  "Email code UI (request, input, states)",
                  "Integrate with session management",
                  "Write tests",
                  "Update documentation",
                ].map((text, index) => (
                  <label className="checkline" key={text}>
                    <input
                      type="checkbox"
                      checked={checks[index]}
                      onChange={() =>
                        setChecks((items) =>
                          items.map((value, i) => (i === index ? !value : value)),
                        )
                      }
                    />
                    {text}
                  </label>
                ))}
              </div>
            </section>
          </div>
        ) : tab === "Review rounds" ? (
          <div className="inline-list">
            {[2, 1].map((round) => (
              <button
                className="round-row"
                key={round}
                onClick={() => go("review", issue.key, String(round))}
              >
                <Icon name="file-text" />
                <span>
                  <strong>Round {round}</strong>
                  <small>
                    {round === 2 ? 3 : 4} findings · Nov {round === 2 ? "5" : "4"}, 2024
                  </small>
                </span>
                <Icon name="chevron-right" />
              </button>
            ))}
          </div>
        ) : (
          <div className="inline-list">
            <Person name="Shanto" />{" "}
            <span className="muted">created this issue and added the initial plan.</span>
          </div>
        )}
        <div className="issue-activity">
          <h2>Activity</h2>
          <p>
            <Person name="Shanto" />{" "}
            <span className="muted">created this issue. Added initial plan and approach.</span>
          </p>
        </div>
      </div>
      <aside className="properties">
        <h3>Status</h3>
        <Select
          label="Status"
          value={issue.status}
          onChange={(value) => onStatus(value as IssueStatus)}
          options={statuses.map((status) => ({ value: status, label: status }))}
        />
        <h3>Assignee</h3>
        <div className="property">
          <Person name={issue.assignee} />
        </div>
        <h3>Milestone</h3>
        <div className="property">
          <Icon name="target" /> &nbsp;{issue.milestone}
        </div>
        <h3>Branch</h3>
        <div className="property">
          <Icon name="git-branch" /> &nbsp; feat/sp-142-email-code
        </div>
        <h3>Pull request</h3>
        <div className="property">
          <Icon name="github" /> &nbsp; <span className="link-blue">#324</span> &nbsp; Add email
          code sign-in
        </div>
        <div className="property-section">
          <div className="section-heading">
            <strong>Review rounds</strong>
            <button className="text-link" onClick={() => go("review", issue.key)}>
              View all →
            </button>
          </div>
          {[2, 1].map((round) => (
            <button
              className="round-row"
              key={round}
              onClick={() => go("review", issue.key, String(round))}
            >
              <Icon name="file-text" />
              <span>
                <strong>Round {round}</strong>
                <small>
                  {round === 2 ? 3 : 4} findings · Nov {round === 2 ? "5" : "4"}, 2024
                </small>
              </span>
              <Icon name="chevron-right" />
            </button>
          ))}
        </div>
        <div className="property-section">
          <div className="section-heading">
            <strong>Labels</strong>
            <button
              className="text-link"
              type="button"
              aria-expanded={editingLabels}
              onClick={() => setEditingLabels(!editingLabels)}
            >
              {editingLabels ? "Done" : "Edit"}
            </button>
          </div>
          <LabelChips labels={issue.labels} />
          {issue.labels.length === 0 && <p className="muted small">No labels</p>}
          {editingLabels && (
            <LabelPicker options={availableLabels} selected={issue.labels} onChange={onLabels} />
          )}
        </div>
      </aside>
    </div>
  );
}
function Review({
  issue,
  findings,
  go,
  decide,
}: {
  issue: Issue;
  findings: Finding[];
  go: (page: Page, issue?: string, round?: string) => void;
  decide: (finding: Finding, status: string) => void;
}) {
  const [selected, setSelected] = useState(0);
  const round = useRouterState({
    select: (state) => state.location.pathname.split("/").at(-1) ?? "2",
  });
  const selectedFinding = findings[selected] ?? findings[0];
  return (
    <>
      <header className="review-header">
        <div className="breadcrumb">
          <button onClick={() => go("issue", issue.key)}>{issue.key}</button> / Reviews / Round{" "}
          {round}
        </div>
        <div className="title-row">
          <div>
            <h1>Review round {round}</h1>
            <p className="issue-meta">
              <Icon name="git-branch" /> &nbsp; a3f9c2d &nbsp; <Icon name="arrow-right" /> &nbsp;
              7b4e1aa　 ·　 <Icon name="clock" /> &nbsp; Nov 5, 2024 – Nov 6, 2024
            </p>
            <p className="issue-summary">
              Adds sign-in with email code, branch creation flow, and related UI updates.
            </p>
          </div>
          <div className="review-actions">
            <Select
              label="Review round"
              value={round}
              onChange={(value) => go("review", issue.key, value)}
              options={[
                { value: "2", label: "Round 2" },
                { value: "1", label: "Round 1" },
              ]}
            />
            <button className="text-button" onClick={() => go("findings")}>
              {findings.filter((item) => item.status === "Open").length} open findings　
              <Icon name="chevron-down" />
            </button>
          </div>
        </div>
      </header>
      <div className="review-split">
        <div>
          <div className="review-tab">
            Findings <span className="count">{findings.length}</span>
          </div>
          {round === "2" ? (
            <div className="finding-cards">
              {findings.map((item, index) => (
                <button
                  className={`finding-card ${index === selected ? "selected" : ""}`}
                  key={item.id}
                  onClick={() => setSelected(index)}
                >
                  <span
                    className={`severity-symbol ${item.severity.toLowerCase().replace(" ", "-")}`}
                  >
                    <Icon name={item.severity === "Nit" ? "info" : "triangle-alert"} />
                  </span>
                  <div className="finding-card-main">
                    <div className="finding-title">
                      <span className="badge">{item.id}</span> <strong>{item.title}</strong>{" "}
                      <Severity value={item.severity} />
                    </div>
                    <p>{item.problem}</p>
                    <div className="finding-footer">
                      <Person name={item.author} />
                      <span className="muted">
                        <Icon name="message-square" /> &nbsp;{index + 1}　{" "}
                        <span className="badge">● {item.status}</span>
                      </span>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          ) : (
            <div className="empty-state">
              <h2>No findings in this preview</h2>
              <p>Choose round 2 to see the example review.</p>
            </div>
          )}
        </div>
        {round === "2" && selectedFinding && (
          <FindingDetail finding={selectedFinding} decide={decide} />
        )}
      </div>
    </>
  );
}
function FindingDetail({
  finding,
  decide,
}: {
  finding: Finding;
  decide: (finding: Finding, status: string) => void;
}) {
  const [reason, setReason] = useState("");
  const [comment, setComment] = useState("");
  const [comments, setComments] = useState<string[]>([]);
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        (e.target as HTMLElement).matches("input, textarea, [role=combobox]") ||
        e.metaKey ||
        e.ctrlKey
      )
        return;
      if (e.key.toLowerCase() === "f") decide(finding, "Fix");
      if (e.key.toLowerCase() === "l") decide(finding, "Later");
      if (e.key.toLowerCase() === "w") setReason(" ");
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [finding, decide]);
  return (
    <aside className="finding-detail">
      <div className="detail-heading">
        <span className="badge">{finding.id}</span>
        <strong>{finding.title}</strong>
        <Severity value={finding.severity} />
        <span className="badge detail-status">● {finding.status}</span>
      </div>
      <p className="detail-author">
        Found by <Person name={finding.author} />{" "}
        <span className="muted"> · &nbsp; Nov 6, 2024</span>
      </p>
      <h3>Problem</h3>
      <p>{finding.problem}</p>
      <h3>Evidence</h3>
      <div className="evidence">
        <Icon name="file-text" /> &nbsp; {finding.file}
        <div className="code-sample">
          42　 <span>const</span> session = <span>await</span> getSession();
          <br />
          43　 if (session.expiresAt &lt; Date.now()) {"{"}
          <br />
          44　　 throw new SessionExpiredError();
        </div>
      </div>
      <h3>Discussion</h3>
      <div className="discussion">
        <Avatar name="Claude Code" />
        <p>
          <strong>Claude Code</strong> <small>Nov 6, 2024, 11:28 AM</small>
          <br />
          We should handle expiration gracefully.
        </p>
      </div>
      <div className="discussion">
        <Avatar name="Shanto" />
        <p>
          <strong>Shanto</strong> <small>Nov 6, 2024, 12:01 PM</small>
          <br />
          Agreed. Let’s show a helpful message if it expires.
        </p>
      </div>
      {comments.map((item, i) => (
        <div className="discussion" key={`${item}-${i}`}>
          <Avatar name="Shanto" />
          <p>
            <strong>Shanto</strong>
            <br />
            {item}
          </p>
        </div>
      ))}
      <form
        className="comment-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (comment.trim()) {
            setComments((items) => [...items, comment.trim()]);
            setComment("");
          }
        }}
      >
        <input
          aria-label="Add a comment"
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          placeholder="Add a comment..."
        />
        <button className="button" disabled={!comment.trim()}>
          Send
        </button>
      </form>
      <h3>Decision</h3>
      <div className="decisions">
        <button onClick={() => decide(finding, "Fix")}>
          <Icon name="check" /> &nbsp; Fix <kbd>F</kbd>
        </button>
        <button onClick={() => setReason(" ")}>
          <Icon name="circle-slash" /> &nbsp; Wontfix <kbd>W</kbd>
        </button>
        <button onClick={() => decide(finding, "Later")}>
          <Icon name="clock" /> &nbsp; Later <kbd>L</kbd>
        </button>
      </div>
      {reason !== "" && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (reason.trim()) {
              decide(finding, "Wontfix");
              setReason("");
            }
          }}
        >
          <label>
            Reason
            <input required value={reason} onChange={(e) => setReason(e.target.value)} />
          </label>
          <button className="button">Confirm wontfix</button>{" "}
          <button type="button" className="text-button" onClick={() => setReason("")}>
            Cancel
          </button>
        </form>
      )}
    </aside>
  );
}
function Findings({
  findings,
  go,
  decide,
}: {
  findings: Finding[];
  go: (page: Page, issue?: string, round?: string) => void;
  decide: (finding: Finding, status: string) => void;
}) {
  const [selected, setSelected] = useState(0);
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState("All");
  const [status, setStatus] = useState("Open");
  const visible = findings.filter(
    (item) =>
      item.title.toLowerCase().includes(query.toLowerCase()) &&
      (severity === "All" || item.severity === severity) &&
      (status === "All" || item.status === status),
  );
  const active = visible[selected] ?? visible[0];
  return (
    <>
      <div className="findings-toolbar">
        <Header
          crumb="Projects / Sparrow"
          title="Open findings"
          description="Findings across review rounds that need attention."
        />
        <div className="findings-filters filters">
          <input
            className="filter-search"
            placeholder="Search findings..."
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelected(0);
            }}
          />
          <Filter
            name="Severity"
            value={severity}
            onChange={setSeverity}
            options={["Blocking", "Should fix", "Nit"]}
          />
          <Filter
            name="Status"
            value={status}
            onChange={setStatus}
            options={["Open", "Fix", "Wontfix", "Later"]}
          />
        </div>
      </div>
      <div className="review-split findings-split">
        <div>
          {visible.length === 0 ? (
            <div className="empty-state">
              <h2>No findings found</h2>
              <p>Try changing your filters.</p>
            </div>
          ) : (
            [...new Set(visible.map((item) => item.issue))].map((key) => (
              <section key={key}>
                <div className="findings-group">
                  <button className="text-link" onClick={() => go("issue", key)}>
                    {key}
                  </button>{" "}
                  <strong>
                    {key === "SP-142"
                      ? "Add email code sign-in"
                      : "Clarify error message for invalid login"}
                  </strong>
                </div>
                {visible
                  .filter((item) => item.issue === key)
                  .map((item) => (
                    <button
                      key={`${item.issue}-${item.id}`}
                      className={`finding-list-row ${item === active ? "selected" : ""}`}
                      onClick={() => setSelected(visible.indexOf(item))}
                    >
                      <Severity value={item.severity} />
                      <span>
                        <strong>{item.title}</strong>
                        <small>{item.problem}</small>
                      </span>
                      <Person name={item.author} />
                      <span className="muted">
                        Round {item.round} · {item.id}
                      </span>
                    </button>
                  ))}
              </section>
            ))
          )}
        </div>
        {active && <FindingDetail finding={active} decide={decide} />}
      </div>
    </>
  );
}
function Activity({ go }: { go: (page: Page, issue?: string) => void }) {
  const [actor, setActor] = useState("All");
  const events = [
    {
      time: "11:28 AM",
      name: "Claude Code",
      action: "started a code review on",
      detail: "Reviewing the sign-in flow and its edge cases.",
    },
    {
      time: "11:31 AM",
      name: "Claude Code",
      action: "added a finding to",
      detail: "Missing validation for branch name.",
    },
    {
      time: "11:34 AM",
      name: "Codex",
      action: "added a finding to",
      detail: "Session can expire during sign-in.",
    },
    {
      time: "11:37 AM",
      name: "Shanto",
      action: "decided to fix a finding on",
      detail: "Let's fix this and add tests.",
    },
    {
      time: "11:52 AM",
      name: "Codex",
      action: "marked a finding fixed on",
      detail: "Implemented the fix and added tests.",
    },
  ];
  return (
    <>
      <Header
        crumb="Activity"
        title="Activity"
        description="Track what’s happening across your projects, issues, and agents."
      />
      <div className="activity-content">
        <div className="filters">
          <Filter
            name="Actor"
            value={actor}
            onChange={setActor}
            options={["Shanto", "Claude Code", "Codex"]}
          />
          <div className="filter-static">Project &nbsp; Sparrow</div>
        </div>
        <h2>Today</h2>
        <div className="timeline">
          {events
            .filter((event) => actor === "All" || event.name === actor)
            .map((event) => (
              <div className="timeline-row" key={event.time}>
                <time>{event.time}</time>
                <div className="timeline-dot" />
                <Person name={event.name} />
                <div>
                  <span>{event.action} </span>
                  <button className="text-link" onClick={() => go("issue", "SP-142")}>
                    SP-142
                  </button>
                  <p className="muted">{event.detail}</p>
                </div>
              </div>
            ))}
        </div>
        <h2>Yesterday</h2>
        <div className="timeline-row">
          <time>4:18 PM</time>
          <div className="timeline-dot" />
          <Person name="Shanto" />
          <div>
            created{" "}
            <button className="text-link" onClick={() => go("issue", "SP-142")}>
              SP-142
            </button>
            <p className="muted">Added the initial plan.</p>
          </div>
        </div>
      </div>
    </>
  );
}
function Milestones({ go }: { go: (page: Page) => void }) {
  return (
    <>
      <Header
        crumb="Projects / Sparrow"
        title="Milestones"
        description="Track the work that matters for your project."
      />
      <div className="settings-content">
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>Launch v1.0</h2>
              <p className="muted">Active · 9 issues</p>
            </div>
            <span className="badge">Active</span>
          </div>
          <p className="muted">The core Super Sparrow workflow, from planning through review.</p>
          <div className="progress">
            <span />
          </div>
          <button className="text-link" onClick={() => go("issues")}>
            View issues →
          </button>
        </section>
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>Later</h2>
              <p className="muted">Planned · 1 issue</p>
            </div>
            <span className="badge">Planned</span>
          </div>
        </section>
      </div>
    </>
  );
}
function Settings({
  page,
  go,
  notify,
}: {
  page: Page;
  go: (page: Page) => void;
  notify: (message: string) => void;
}) {
  const [dialog, setDialog] = useState<"member" | "token" | null>(null);
  const [agent, setAgent] = useState("Claude Code");
  const [name, setName] = useState("");
  const [members, setMembers] = useState(["Shanto", "Alex", "Jamie"]);
  const [tokens, setTokens] = useState(["Claude Code · laptop", "Codex · laptop"]);
  const add = (e: FormEvent) => {
    e.preventDefault();
    if (dialog === "member") setMembers((items) => [...items, name]);
    else setTokens((items) => [...items, name]);
    setName("");
    setDialog(null);
    notify("Added to this preview");
  };
  return (
    <>
      <header className="page-header">
        <div className="breadcrumb">Settings</div>
        <h1>{page === "sessions" ? "Signed-in devices" : "People and agents"}</h1>
        <p className="muted">
          {page === "sessions"
            ? "Manage your active sessions."
            : "Manage your team members, their agents, and agent tokens."}
        </p>
        <div className="tabs">
          <button className={page === "settings" ? "selected" : ""} onClick={() => go("settings")}>
            <Icon name="users" /> &nbsp; People and agents
          </button>
          <button className={page === "sessions" ? "selected" : ""} onClick={() => go("sessions")}>
            <Icon name="monitor" /> &nbsp; Signed-in devices
          </button>
        </div>
      </header>
      <div className="settings-content">
        {page === "sessions" ? (
          <section className="panel">
            <h2>Signed-in devices</h2>
            <div className="data-row">
              <div>
                <strong>Current browser</strong> <span className="badge">Current</span>
                <div className="muted small">Last seen just now</div>
              </div>
              <button className="button" onClick={() => notify("This is a frontend preview")}>
                Sign out
              </button>
            </div>
          </section>
        ) : (
          <>
            <section className="panel">
              <div className="panel-heading">
                <div>
                  <h2>Team members</h2>
                  <p className="muted">{members.length} team members · 4 agents</p>
                </div>
                <button className="primary" onClick={() => setDialog("member")}>
                  <Icon name="plus" /> &nbsp; Add member
                </button>
              </div>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Member</th>
                      <th>Agents</th>
                      <th>Role</th>
                      <th>Joined</th>
                    </tr>
                  </thead>
                  <tbody>
                    {members.map((member, index) => (
                      <tr key={member}>
                        <td>
                          <div className="person">
                            <Avatar name={member} />
                            <div>
                              <strong>{member}</strong>
                              <div className="muted small">
                                {member.toLowerCase()}@supersparrow.com
                              </div>
                            </div>
                          </div>
                        </td>
                        <td>
                          <div className="agent-list">
                            {(index === 0
                              ? ["Claude Code", "Codex"]
                              : index === 1
                                ? ["Claude Code"]
                                : index === 2
                                  ? ["Codex"]
                                  : []
                            ).map((agent) => (
                              <span className="agent-pill" key={agent}>
                                <Avatar name={agent} />
                                {agent}
                                <span className="dot" />
                                Active
                              </span>
                            ))}
                          </div>
                        </td>
                        <td>
                          <span className="badge">{index === 0 ? "Owner" : "Member"}</span>
                        </td>
                        <td className="muted">Nov 6, 2024</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
            <section className="panel">
              <div className="panel-heading">
                <div>
                  <h2>Agent tokens</h2>
                  <p className="muted">Manage the tokens your agents use to connect.</p>
                </div>
                <button className="primary" onClick={() => setDialog("token")}>
                  <Icon name="plus" /> &nbsp; Create token
                </button>
              </div>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Owned by</th>
                      <th>Agent</th>
                      <th>Expires</th>
                      <th>Last used</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tokens.map((token, index) => (
                      <tr key={token}>
                        <td>
                          <strong>{token}</strong>
                        </td>
                        <td>Shanto</td>
                        <td>
                          <Person name={index === 1 ? "Codex" : "Claude Code"} />
                        </td>
                        <td>Dec 5, 2024</td>
                        <td>Nov 6, 2024</td>
                        <td>
                          <button
                            className="text-button"
                            onClick={() => {
                              setTokens((items) => items.filter((item) => item !== token));
                              notify("Token removed from this preview");
                            }}
                          >
                            Revoke
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          </>
        )}
      </div>
      {dialog && (
        <div className="overlay">
          <div className="modal" role="dialog" aria-modal="true">
            <form onSubmit={add}>
              <h2>{dialog === "member" ? "Add member" : "Create agent token"}</h2>
              <label>
                {dialog === "member" ? "Name" : "Token label"}
                <input autoFocus required value={name} onChange={(e) => setName(e.target.value)} />
              </label>
              {dialog === "member" && (
                <label>
                  Email
                  <input type="email" required />
                </label>
              )}
              {dialog === "token" && (
                <div className="select-field">
                  <span>Agent</span>
                  <Select
                    label="Agent"
                    value={agent}
                    onChange={setAgent}
                    options={[
                      { value: "Claude Code", label: "Claude Code" },
                      { value: "Codex", label: "Codex" },
                    ]}
                  />
                </div>
              )}
              <div className="modal-actions">
                <button className="button" type="button" onClick={() => setDialog(null)}>
                  Cancel
                </button>
                <button className="primary">
                  {dialog === "member" ? "Add member" : "Create token"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
