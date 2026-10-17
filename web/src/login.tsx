import { useState } from "react";
import { ArrowRight } from "lucide-react";
import { Brand } from "./components/brand";
import { errorMessage, request, type Session } from "./api";

export function Login({
  session,
  onLogin,
}: {
  session: Session;
  onLogin: (s: Session) => void;
}) {
  const [error, setError] = useState("");
  const setup = session.mode === "local" && session.setupRequired;
  const recoveryPage =
    window.location.pathname === new URL("login", document.baseURI).pathname;
  const showLocal =
    session.localLogin && (session.mode === "local" || recoveryPage);
  return (
    <div className="login-page">
      <div className="login-card">
        <Brand className="login-brand" />
        <h1>{setup ? "Create administrator" : "Sign in"}</h1>
        {error && (
          <div className="alert" role="alert">
            {error}
          </div>
        )}
        {session.mode === "oidc" && !recoveryPage && (
          <a
            className="primary full"
            href={new URL("auth/login", document.baseURI).toString()}
          >
            Continue with your identity provider <ArrowRight size={16} />
          </a>
        )}
        {showLocal && (
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              const data = new FormData(e.currentTarget);
              if (setup && data.get("password") !== data.get("confirmation")) {
                setError("Passwords do not match.");
                return;
              }
              try {
                await request(setup ? "api/setup" : "api/login", {
                  method: "POST",
                  body: JSON.stringify({
                    username: data.get("username"),
                    password: data.get("password"),
                  }),
                });
                const currentSession = await request<Session>("api/session");
                window.history.replaceState(
                  null,
                  "",
                  new URL(".", document.baseURI),
                );
                onLogin(currentSession);
              } catch (err) {
                setError(errorMessage(err));
              }
            }}
          >
            <label>
              Username
              <input name="username" autoComplete="username" required />
            </label>
            <label>
              Password
              <input
                type="password"
                name="password"
                autoComplete={setup ? "new-password" : "current-password"}
                minLength={setup ? 12 : undefined}
                required
              />
            </label>
            {setup && (
              <label>
                Confirm password
                <input
                  type="password"
                  name="confirmation"
                  autoComplete="new-password"
                  minLength={12}
                  required
                />
              </label>
            )}
            <button className="primary full">
              {setup ? "Create administrator" : "Sign in"}{" "}
              <ArrowRight size={16} />
            </button>
          </form>
        )}
        {session.mode !== "local" && session.localLogin && !recoveryPage && (
          <a
            className="text-button"
            href={new URL("login", document.baseURI).toString()}
          >
            Local sign in <ArrowRight size={16} />
          </a>
        )}
      </div>
    </div>
  );
}
