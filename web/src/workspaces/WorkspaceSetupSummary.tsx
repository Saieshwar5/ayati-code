interface WorkspaceSetupSummaryProps {
  project: string;
  branch: string;
  environmentCount: number;
  hasSetupCommand: boolean;
}

interface WorkspaceCreateActionProps extends WorkspaceSetupSummaryProps {
  canSubmit: boolean;
  submitting: boolean;
}

export function WorkspaceCreateAction(props: WorkspaceCreateActionProps) {
  const outcome = props.canSubmit
    ? `Workspace on ${props.branch}`
    : "Choose a project and branch to continue";
  return (
    <footer className="workspace-create-final" aria-labelledby="workspace-create-final-title">
      <div>
        <h2 id="workspace-create-final-title">{props.project || "Workspace setup"}</h2>
        <p>{outcome}</p>
        <div className="final-workspace-facts">
          <span>{props.hasSetupCommand ? "Custom setup" : "Automatic setup"}</span>
          <span>{props.environmentCount ? `${props.environmentCount} variables` : "No variables"}</span>
        </div>
      </div>
      <button className="primary create-workspace-action" type="submit" disabled={!props.canSubmit || props.submitting}>
        {props.submitting ? "Creating workspace…" : "Create workspace"}
      </button>
    </footer>
  );
}
