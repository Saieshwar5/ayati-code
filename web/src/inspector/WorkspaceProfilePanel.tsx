import type { ProjectProfile, Workspace } from "../api/contracts";

interface WorkspaceProfilePanelProps {
  workspace: Workspace;
}

export function WorkspaceProfilePanel({ workspace }: WorkspaceProfilePanelProps) {
  const profile = workspace.project_profile;
  return (
    <section className="inspector-panel active" role="tabpanel">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Prepared project</p>
          <h3>Workspace profile</h3>
        </div>
        <span className={`status ${workspace.status}`}>{workspace.status.replaceAll("_", " ")}</span>
      </div>
      <div className="profile-summary">
        <ProfileFact label="Preparation" value={preparationLabel(workspace)} />
      </div>
      {profile ? <ResolvedProfile profile={profile} /> : (
        <p className="profile-empty">Project facts will appear as deterministic analysis completes.</p>
      )}
    </section>
  );
}

function ResolvedProfile({ profile }: { profile: ProjectProfile }) {
  const commands = [
    ["Setup", profile.setup_command],
    ["Test", profile.test_command],
    ["Lint", profile.lint_command],
    ["Typecheck", profile.typecheck_command],
    ["Build", profile.build_command],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));

  return (
    <>
      <div className="profile-section">
        <p className="profile-section-title">Detected project</p>
        <ProfileFact label="Root" value={profile.project_root} code />
        <ProfileFact label="Languages" value={join(profile.languages)} />
        <ProfileFact label="Runtimes" value={join(profile.runtime_versions)} />
        <ProfileFact label="Package managers" value={join(profile.package_managers)} />
        <ProfileFact label="Lockfiles" value={join(profile.lockfiles)} />
        {profile.instructions_file && <ProfileFact label="Instructions" value={profile.instructions_file} code />}
      </div>
      <div className="profile-section">
        <p className="profile-section-title">Baseline</p>
        <ProfileFact label="Commit" value={profile.baseline_commit?.slice(0, 12) || "Not recorded"} code />
        <ProfileFact label="Dependency setup" value={profile.setup_result} />
        <ProfileFact label="Git baseline" value={profile.baseline_result} />
      </div>
      {commands.length > 0 && (
        <div className="profile-section">
          <p className="profile-section-title">Resolved commands</p>
          <div className="profile-commands">
            {commands.map(([label, command]) => (
              <div key={label}>
                <span>{label}</span>
                <code>{command}</code>
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

function ProfileFact({ label, value, code = false }: { label: string; value: string; code?: boolean }) {
  return (
    <div className="profile-fact">
      <span>{label}</span>
      {code ? <code>{value}</code> : <strong>{value}</strong>}
    </div>
  );
}

function preparationLabel(workspace: Workspace): string {
  if (workspace.preparation_detail) return workspace.preparation_detail;
  return workspace.preparation_stage.replaceAll("_", " ");
}

function join(values: string[]): string {
  return values.length ? values.join(" · ") : "Not detected";
}
