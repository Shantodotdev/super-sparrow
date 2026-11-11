import {
  Activity,
  ArrowRight,
  Bird,
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  CircleCheck,
  CircleDashed,
  CircleDot,
  CircleSlash,
  CircleUserRound,
  CircleX,
  Clock,
  Copy,
  FileText,
  Folder,
  GitBranch,
  Github,
  Inbox,
  Info,
  Keyboard,
  Link as LinkIcon,
  List,
  ListFilter,
  ListTodo,
  MessageSquare,
  Monitor,
  Plus,
  Search,
  Settings,
  SquareCheck,
  Tag,
  Target,
  Terminal,
  TriangleAlert,
  Users,
} from "lucide-react";

const icons = {
  activity: Activity,
  "arrow-right": ArrowRight,
  bird: Bird,
  check: Check,
  "chevron-down": ChevronDown,
  "chevron-right": ChevronRight,
  circle: Circle,
  "circle-check": CircleCheck,
  "circle-dashed": CircleDashed,
  "circle-dot": CircleDot,
  "circle-slash": CircleSlash,
  "circle-user-round": CircleUserRound,
  "circle-x": CircleX,
  clock: Clock,
  copy: Copy,
  "file-text": FileText,
  folder: Folder,
  "git-branch": GitBranch,
  github: Github,
  inbox: Inbox,
  info: Info,
  keyboard: Keyboard,
  link: LinkIcon,
  list: List,
  "list-filter": ListFilter,
  "list-todo": ListTodo,
  "message-square": MessageSquare,
  monitor: Monitor,
  plus: Plus,
  search: Search,
  settings: Settings,
  "square-check": SquareCheck,
  tag: Tag,
  target: Target,
  terminal: Terminal,
  "triangle-alert": TriangleAlert,
  users: Users,
} as const;

export type IconName = keyof typeof icons;

/** Displays a decorative icon from Lucide. */
export function Icon({ name, className = "" }: { name: IconName; className?: string }) {
  const LucideIcon = icons[name];

  return (
    <LucideIcon
      aria-hidden="true"
      className={`icon inline-block size-[17px] flex-none align-[-3px] [&.plan-icon]:h-6 [&.plan-icon]:w-6 [&.project-logo]:h-[19px] [&.project-logo]:w-[19px] [&.status-backlog]:text-[#a9b6c8] [&.status-done]:text-[#44ce91] [&.status-icon]:text-[17px] [&.status-icon]:text-[#98a9c3] [&.status-in-progress]:text-[#4c7aff] [&.status-in-review]:text-[#a574ff] ${className}`}
    />
  );
}
