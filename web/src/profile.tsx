import { CredentialKind } from "./domain";
import type { ThemePreference } from "./components/theme-picker";
import type { Session } from "./api";
/** ProfileSettings displays credential metadata and session-specific appearance settings. */
export function ProfileSettings({
  theme,
  session,
}: {
  /** Session appearance preference and its update controls. */
  theme: ThemePreference;
  /** Authenticated credential metadata and administrative scopes. */
  session: Session;
}) {
  return (
    <>
      <section className="panel settings-panel">
        <div className="panel-head">
          <h2>Credential</h2>
        </div>
        <div className="settings-form">
          <label>
            Signed in as
            <input value={session.user} disabled />
          </label>
          <p className="settings-note">
            {session.mode === CredentialKind.Admin
              ? "Garage admin token"
              : "Garage S3 access key"}
          </p>
          {session.mode === CredentialKind.Admin && (
            <p className="settings-note">
              Allowed operations: {session.scope?.join(", ")}
            </p>
          )}
        </div>
      </section>
      <section className="panel settings-panel">
        <div className="panel-head">
          <h2>Appearance</h2>
        </div>
        <div className="settings-form">
          <label>
            Theme
            <select
              value={theme.selected}
              disabled={!theme.names.length || theme.busy}
              onChange={(event) => void theme.change(event.target.value)}
            >
              {theme.names.map((name) => (
                <option key={name}>{name}</option>
              ))}
            </select>
          </label>
          <p className="settings-note">
            Appearance is saved in this browser and your current session.
          </p>
          {theme.error && (
            <p role="alert" className="object-error">
              {theme.error}
            </p>
          )}
        </div>
      </section>
    </>
  );
}
