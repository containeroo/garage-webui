import type { ThemePreference } from "./components/theme-picker";
import { errorMessage } from "./api";
import { useEffect, useState } from "react";

type Request = <T>(path: string, options?: RequestInit) => Promise<T>;
type Profile = {
  username: string;
  email: string;
  displayName: string;
  localCredentialEnabled: boolean;
};

// ProfileSettings lets a user manage their own profile and local password.
export function ProfileSettings({
  request,
  localLogin,
  theme,
}: {
  request: Request;
  localLogin: boolean;
  theme: ThemePreference;
}) {
  const [profile, setProfile] = useState<Profile>();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    request<Profile>("api/profile")
      .then(setProfile)
      .catch((err) => setError(errorMessage(err)));
  }, [request]);
  if (!profile)
    return <div className="alert">{error || "Loading profile…"}</div>;
  return (
    <>
      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}
      {saved && <div className="badge">Profile saved</div>}
      <section className="panel user-list">
        <div className="panel-head">
          <div>
            <h2>Profile</h2>
          </div>
        </div>
        <form
          className="user-form"
          onSubmit={async (event) => {
            event.preventDefault();
            try {
              setError("");
              await request("api/profile", {
                method: "PUT",
                body: JSON.stringify({
                  email: profile.email,
                  displayName: profile.displayName,
                }),
              });
              setSaved(true);
            } catch (err) {
              setError(errorMessage(err));
            }
          }}
        >
          <label>
            Username
            <input value={profile.username} disabled />
          </label>
          <label>
            Display name
            <input
              maxLength={128}
              value={profile.displayName}
              onChange={(event) =>
                setProfile({ ...profile, displayName: event.target.value })
              }
            />
          </label>
          <label>
            Email
            <input
              type="email"
              value={profile.email}
              onChange={(event) =>
                setProfile({ ...profile, email: event.target.value })
              }
            />
          </label>
          <button className="primary">Save profile</button>
        </form>
      </section>
      <section className="panel user-list">
        <div className="panel-head">
          <h2>Appearance</h2>
        </div>
        <div className="user-form">
          <label>
            Theme
            <select
              value={theme.selected}
              disabled={!theme.names.length || theme.busy}
              onChange={(event) => void theme.change(event.target.value)}
            >
              {theme.names.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          {theme.error && (
            <p className="object-error" role="alert">
              {theme.error}
            </p>
          )}
        </div>
      </section>
      {localLogin && profile.localCredentialEnabled && (
        <section className="panel user-list">
          <div className="panel-head">
            <div>
              <h2>Local password</h2>
            </div>
          </div>
          <form
            className="user-form"
            onSubmit={async (event) => {
              event.preventDefault();
              try {
                setError("");
                await request("api/profile/password", {
                  method: "POST",
                  body: JSON.stringify({ current, password }),
                });
                window.location.reload();
              } catch (err) {
                setError(errorMessage(err));
              }
            }}
          >
            <label>
              Current password
              <input
                type="password"
                autoComplete="current-password"
                required
                value={current}
                onChange={(event) => setCurrent(event.target.value)}
              />
            </label>
            <label>
              New password
              <input
                type="password"
                autoComplete="new-password"
                required
                minLength={12}
                maxLength={72}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </label>
            <button className="secondary">Change password</button>
          </form>
        </section>
      )}
    </>
  );
}
