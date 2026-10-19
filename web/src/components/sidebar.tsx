import {
  ChevronRight,
  Database,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Server,
  ShieldCheck,
} from "lucide-react";
import { allows, type Health, type Page, type Session } from "../api";
import { Brand } from "./brand";

// Sidebar contains the main navigation and the current account.
export function Sidebar({
  session,
  page,
  health,
  onNavigate,
  onSignOut,
}: {
  session: Session;
  page: Page;
  health?: Health;
  onNavigate: (page: Page) => void;
  onSignOut: () => void;
}) {
  return (
    <aside className="sidebar">
      <Brand />
      <div className="nav-label">WORKSPACE</div>
      <nav>
        {(
          [
            ["overview", LayoutDashboard, "Overview"],
            ["cluster", Server, "Cluster"],
            ["buckets", Database, "Buckets"],
            ["keys", KeyRound, "Access keys"],
          ] as const
        )
          .filter(
            ([id]) =>
              id === "buckets" ||
              (id === "overview" && session.mode === "admin") ||
              (id === "cluster" && allows(session, "GetClusterStatus")) ||
              (id === "keys" && allows(session, "ListKeys")),
          )
          .map(([id, Icon, label]) => (
            <button
              key={id}
              className={page === id ? "active" : ""}
              aria-current={page === id ? "page" : undefined}
              onClick={() => {
                onNavigate(id);
              }}
            >
              <Icon size={17} />
              {label}
              {page === id && <ChevronRight size={15} className="nav-arrow" />}
            </button>
          ))}
        {allows(session, "ListAdminTokens") && (
          <button
            className={page === "tokens" ? "active" : ""}
            aria-current={page === "tokens" ? "page" : undefined}
            onClick={() => {
              onNavigate("tokens");
            }}
          >
            <ShieldCheck size={17} />
            Admin tokens
            {page === "tokens" && (
              <ChevronRight size={15} className="nav-arrow" />
            )}
          </button>
        )}
      </nav>
      <div className="sidebar-bottom">
        {session.mode === "admin" && (
          <div className="sidebar-status">
            <span
              className={`dot ${health?.status === "healthy" ? "" : "warning"}`}
            />
            <div>
              <strong>
                {health?.status === "healthy"
                  ? "System operational"
                  : "System status"}
              </strong>
              <small>{health?.status || "Unavailable"}</small>
            </div>
          </div>
        )}
        <div className="profile">
          <button
            className="profile-link"
            onClick={() => onNavigate("profile")}
            aria-label={`My profile: ${session.user}`}
            aria-current={page === "profile" ? "page" : undefined}
          >
            <span className="avatar">
              {session.user.slice(0, 1).toUpperCase()}
            </span>
            <span>
              <strong>{session.user}</strong>
              <small>{session.mode}</small>
            </span>
          </button>
          {
            <button title="Sign out" aria-label="Sign out" onClick={onSignOut}>
              <LogOut size={16} />
            </button>
          }
        </div>
      </div>
    </aside>
  );
}
