import { useEffect, useState } from "react";
import type { Changes, PublishInput, Workspace } from "../api/contracts";
import { api } from "../api/client";
import { PublishPanel } from "../inspector/PublishPanel";
import { Icon } from "../ui/Icon";
import { countChangedFiles, WorkspaceChangesReview } from "./WorkspaceChangesReview";

interface WorkspaceChangesDrawerProps {
  workspace: Workspace;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onWorkspaceUpdate: (workspace: Workspace) => void;
}

const emptyChanges: Changes = { status: "", diff: "" };

export function WorkspaceChangesDrawer(props: WorkspaceChangesDrawerProps) {
  const [changes, setChanges] = useState<Changes>(emptyChanges);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const count = countChangedFiles(changes);

  useEffect(() => {
    setChanges(emptyChanges);
    setError("");
    void loadChanges();
  }, [props.workspace.id]);

  async function loadChanges() {
    if (props.workspace.status !== "ready") return;
    setLoading(true);
    setError("");
    try {
      setChanges(await api.changes(props.workspace.id));
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function publish(input: PublishInput) {
    setPublishing(true);
    try {
      const updated = await api.publish(props.workspace.id, input);
      props.onWorkspaceUpdate(updated);
      await loadChanges();
      setPublishOpen(false);
      return true;
    } finally {
      setPublishing(false);
    }
  }

  function toggle() {
    const next = !props.open;
    props.onOpenChange(next);
    if (next) void loadChanges();
  }

  return (
    <>
      {props.open && (
        <aside className="workspace-changes-drawer" aria-label="Workspace changes">
          <WorkspaceChangesReview
            embedded
            changes={changes}
            loading={loading}
            error={error}
            onClose={() => props.onOpenChange(false)}
            onRefresh={() => void loadChanges()}
            onPublish={() => setPublishOpen(true)}
          />
        </aside>
      )}
      <nav className="workspace-changes-rail" aria-label="Conversation tools">
        <button className={props.open ? "active" : ""} type="button" aria-label={`Changes${count ? `, ${count} files` : ""}`} aria-pressed={props.open} onClick={toggle}>
          <Icon name="changes" />
          {count > 0 && <span>{count > 99 ? "99+" : count}</span>}
        </button>
      </nav>
      {publishOpen && (
        <div className="workspace-dialog-backdrop" role="presentation" onMouseDown={() => setPublishOpen(false)}>
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-label="Publish workspace changes" onMouseDown={(event) => event.stopPropagation()}>
            <button className="dialog-close" type="button" aria-label="Close publish dialog" onClick={() => setPublishOpen(false)}>×</button>
            <PublishPanel workspace={props.workspace} publishing={publishing} onPublish={publish} />
          </section>
        </div>
      )}
    </>
  );
}
