import type { FormEvent } from "react";
import { useState } from "react";
import { ArrowRight } from "lucide-react";
import { Brand } from "./components/brand";
import { errorMessage, request, type Session } from "./api";
/** Login verifies Garage admin or S3 credentials and opens the resulting session. */
export function Login({
  onLogin,
}: {
  /** Replaces session state after successful credential verification. */
  onLogin: (session: Session) => void;
}) {
  const [kind, setKind] = useState("admin");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // submitLogin verifies the selected Garage credentials and retrieves the resulting session.
  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const data = new FormData(event.currentTarget);
    try {
      await request("api/login", {
        method: "POST",
        body: JSON.stringify(
          kind === "admin"
            ? { kind, token: data.get("token") }
            : {
                kind,
                accessKeyId: data.get("accessKeyId"),
                secretAccessKey: data.get("secretAccessKey"),
              },
        ),
      });
      onLogin(await request<Session>("api/session"));
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <Brand className="login-brand" />
        <h1>Sign in to Garage</h1>
        <div className="bucket-tabs" role="tablist" aria-label="Sign-in method">
          {[
            ["admin", "Admin token"],
            ["s3", "S3 credentials"],
          ].map(([value, label]) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={kind === value}
              onClick={() => {
                setKind(value);
                setError("");
              }}
            >
              {label}
            </button>
          ))}
        </div>
        {error && (
          <div className="alert" role="alert">
            {error}
          </div>
        )}
        <form key={kind} onSubmit={submitLogin}>
          {kind === "admin" ? (
            <label>
              Admin token
              <input
                name="token"
                type="password"
                autoComplete="current-password"
                required
              />
            </label>
          ) : (
            <>
              <label>
                Access key ID
                <input name="accessKeyId" autoComplete="username" required />
              </label>
              <label>
                Secret access key
                <input
                  name="secretAccessKey"
                  type="password"
                  autoComplete="current-password"
                  required
                />
              </label>
            </>
          )}
          <p className="settings-note">
            {kind === "admin"
              ? "Use a Garage admin token. Its scopes determine your access."
              : "Use an S3 key to browse its accessible buckets. Garage enforces read and write permissions."}
          </p>
          <button className="primary full" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
            <ArrowRight size={16} />
          </button>
        </form>
      </div>
    </div>
  );
}
