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
  FileText,
  Folder,
  GitBranch,
  Github,
  Info,
  List,
  ListFilter,
  ListTodo,
  MessageSquare,
  Monitor,
  Plus,
  Search,
  Settings,
  SquareCheck,
  Target,
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
  "file-text": FileText,
  folder: Folder,
  "git-branch": GitBranch,
  github: Github,
  info: Info,
  list: List,
  "list-filter": ListFilter,
  "list-todo": ListTodo,
  "message-square": MessageSquare,
  monitor: Monitor,
  plus: Plus,
  search: Search,
  settings: Settings,
  "square-check": SquareCheck,
  target: Target,
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
