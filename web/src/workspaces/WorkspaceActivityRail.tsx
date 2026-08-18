import type { WorkspaceStatus } from "../api/contracts";
import { Icon, type IconName } from "../ui/Icon";
import type { WorkspacePanelSection } from "./WorkspacePanel";

interface WorkspaceActivityRailProps {
  open: boolean;
  selected: WorkspacePanelSection;
  taskCount: number;
  changeCount: number;
  workspaceStatus: WorkspaceStatus;
  onSelect: (section: WorkspacePanelSection) => void;
}

const tools: Array<{ section: WorkspacePanelSection; label: string; icon: IconName }> = [
  { section: "tasks", label: "Tasks", icon: "tasks" },
  { section: "changes", label: "Changes", icon: "changes" },
  { section: "workspace", label: "Workspace", icon: "details" },
];

export function WorkspaceActivityRail(props: WorkspaceActivityRailProps) {
  return (
    <nav className="workspace-activity-rail" aria-label="Workspace tools">
      {tools.map((tool) => {
        const active = props.open && props.selected === tool.section;
        const count = tool.section === "tasks" ? props.taskCount : tool.section === "changes" ? props.changeCount : 0;
        const attention = tool.section === "workspace" && needsAttention(props.workspaceStatus);
        return (
          <button
            className={`${active ? "active" : ""}${attention ? " attention" : ""}`}
            type="button"
            key={tool.section}
            title={tool.label}
            aria-label={tool.label}
            aria-pressed={active}
            onClick={() => props.onSelect(tool.section)}
          >
            <Icon name={tool.icon} />
            {count > 0 && <span>{count > 99 ? "99+" : count}</span>}
            {attention && <i aria-hidden="true" />}
          </button>
        );
      })}
    </nav>
  );
}

function needsAttention(status: WorkspaceStatus): boolean {
  return status !== "ready";
}
