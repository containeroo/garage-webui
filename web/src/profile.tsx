import type { ThemePreference } from "./components/theme-picker";
import type { Session } from "./api";
export function ProfileSettings({
  theme,
  session,
}: {
  theme: ThemePreference;
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
            {session.mode === "admin"
              ? "Garage admin token"
              : "Garage S3 access key"}
          </p>
          {session.mode === "admin" && (
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
            Appearance is saved for this browser session.
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
