import { useQuestions } from "./components/ui/questions";
import { errorMessage } from "./api";
import { useCallback, useEffect, useRef, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { GroupManagement } from "./components/groups";
import { DialogPanel } from "./components/ui/dialog";

type Identity = { provider: string; issuer: string; subject: string };
type Account = {
  username: string;
  role: string;
  email: string;
  displayName: string;
  emailOverridden: boolean;
  displayNameOverridden: boolean;
  groups: string[];
  externalGroups: string[];
  enabled: boolean;
  hasPassword: boolean;
  localCredentialEnabled: boolean;
  identities: Identity[];
};
type PendingIdentity = Identity & { username: string; status: string };
type GroupMapping = { externalGroup: string; group: string };
type Request = <T>(path: string, options?: RequestInit) => Promise<T>;

// UserManagement manages local accounts and their external identity bindings.
export function UserManagement({
  request,
  currentUser,
}: {
  request: Request;
  currentUser: string;
}) {
  const { confirm, prompt } = useQuestions();
  const [users, setUsers] = useState<Account[]>([]);
  const [registration, setRegistration] = useState(false);
  const [pending, setPending] = useState<PendingIdentity[]>([]);
  const [groups, setGroups] = useState<string[]>([]);
  const [mappings, setMappings] = useState<GroupMapping[]>([]);
  const [memberGroups, setMemberGroups] = useState<string[]>([]);
  const [pendingTarget, setPendingTarget] = useState<Record<string, string>>(
    {},
  );
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<Account>();
  const returnFocus = useRef<HTMLElement | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [provider, setProvider] = useState("proxy");
  const [issuer, setIssuer] = useState("");
  const [subject, setSubject] = useState("");
  const [role, setRole] = useState("viewer");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");

  const load = useCallback(async () => {
    const [accounts, policy, waiting, groupList, groupMappings] =
      await Promise.all([
        request<Account[]>("api/users"),
        request<{ enabled: boolean }>("api/registration"),
        request<PendingIdentity[]>("api/pending-identities"),
        request<string[]>("api/groups"),
        request<GroupMapping[]>("api/group-mappings"),
      ]);
    setUsers(accounts);
    setRegistration(policy.enabled);
    setPending(waiting);
    setGroups(groupList);
    setMappings(groupMappings);
    setSelected(
      (previous) =>
        previous &&
        accounts.find((user) => user.username === previous.username),
    );
  }, [request]);

  useEffect(() => {
    load().catch((err) => setError(errorMessage(err)));
  }, [load]);

  async function change(
    path: string,
    method: string,
    body: unknown,
    reloadAfter = false,
  ) {
    try {
      setError("");
      await request(path, { method, body: JSON.stringify(body) });
      if (reloadAfter) {
        window.location.reload();
        return true;
      }
      await load();
      return true;
    } catch (err) {
      setError(errorMessage(err));
      return false;
    }
  }

  const accountPath = (name: string) => `api/users/${encodeURIComponent(name)}`;

  return (
    <>
      {error && !creating && !selected && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}
      <section className="panel user-settings">
        <div className="panel-head">
          <div>
            <h2>External registration</h2>
          </div>
          <label className="user-toggle">
            <input
              type="checkbox"
              checked={registration}
              onChange={(event) =>
                void change("api/registration", "PUT", {
                  enabled: event.target.checked,
                })
              }
            />{" "}
            Enabled
          </label>
        </div>
        <p className="user-note">
          The configured administrator group grants elevated access. Existing
          names are never linked automatically.
        </p>
      </section>
      {pending.length > 0 && (
        <section className="panel user-list">
          <div className="panel-head">
            <div>
              <h2>
                Pending OIDC identities{" "}
                <span className="count">{pending.length}</span>
              </h2>
            </div>
          </div>
          {pending.map((identity) => {
            const key = JSON.stringify([identity.issuer, identity.subject]);
            const target = pendingTarget[key] || "";
            const binding = {
              provider: identity.provider,
              issuer: identity.issuer,
              subject: identity.subject,
            };
            return (
              <div className="user-row" key={key}>
                <div>
                  <strong>
                    {identity.username} · {identity.status}
                  </strong>
                  <small>
                    {identity.issuer} · {identity.subject}
                  </small>
                </div>
                {identity.status === "rejected" ? (
                  <button
                    className="secondary"
                    onClick={() =>
                      void change(
                        "api/pending-identities/reopen",
                        "POST",
                        binding,
                      )
                    }
                  >
                    Reopen
                  </button>
                ) : (
                  <>
                    <select
                      aria-label={`Account for ${identity.username}`}
                      value={target}
                      onChange={(event) =>
                        setPendingTarget({
                          ...pendingTarget,
                          [key]: event.target.value,
                        })
                      }
                    >
                      <option value="">Choose account</option>
                      {users.map((user) => (
                        <option key={user.username} value={user.username}>
                          {user.username}
                        </option>
                      ))}
                    </select>
                    <button
                      className="secondary"
                      disabled={!target}
                      onClick={() =>
                        void change(
                          `${accountPath(target)}/identities`,
                          "POST",
                          binding,
                        )
                      }
                    >
                      Link
                    </button>
                    <button
                      className="secondary"
                      onClick={() =>
                        void change("api/pending-identities/approve", "POST", {
                          identity: binding,
                          username: identity.username,
                        })
                      }
                    >
                      Create account
                    </button>
                    <button
                      className="icon-button danger"
                      title="Reject identity"
                      onClick={async () => {
                        if (await confirm("Reject this identity?"))
                          void change(
                            "api/pending-identities",
                            "DELETE",
                            binding,
                          );
                      }}
                    >
                      <Trash2 size={16} />
                    </button>
                  </>
                )}
              </div>
            );
          })}
        </section>
      )}
      <section className="panel user-list">
        <div className="panel-head">
          <div>
            <h2>
              Local accounts <span className="count">{users.length}</span>
            </h2>
          </div>
          <button
            className="primary"
            onClick={(event) => {
              returnFocus.current = event.currentTarget;
              setError("");
              setCreating(true);
              setSelected(undefined);
              setUsername("");
              setPassword("");
            }}
          >
            <Plus size={16} /> Create user
          </button>
        </div>
        {users.map((user) => (
          <div className="user-row" key={user.username}>
            <div>
              <strong>{user.displayName || user.username}</strong>{" "}
              {user.username === currentUser && (
                <span className="badge">You</span>
              )}
              <small>
                {user.username} · {user.role} · {user.identities.length} linked
                identities ·{" "}
                {user.hasPassword ? "Local password" : "No local password"}
              </small>
            </div>
            <span className="badge">
              {user.enabled ? "Active" : "Disabled"}
            </span>
            <button
              className="secondary"
              onClick={(event) => {
                returnFocus.current = event.currentTarget;
                setError("");
                setSelected(user);
                setCreating(false);
                setUsername(user.username);
                setPassword("");
                setSubject("");
                setRole(user.role);
                setEmail(user.email);
                setDisplayName(user.displayName);
                setMemberGroups(user.groups);
              }}
            >
              Manage
            </button>
          </div>
        ))}
      </section>
      <GroupManagement groups={groups} mappings={mappings} change={change} />
      {(creating || selected) && (
        <DialogPanel
          className="user-modal"
          title={creating ? "Create user" : `Manage ${selected?.username}`}
          returnFocus={returnFocus.current}
          onClose={() => {
            setCreating(false);
            setSelected(undefined);
            setError("");
          }}
        >
          {error && (
            <div className="alert" role="alert">
              {error}
            </div>
          )}
          {creating ? (
            <form
              className="user-form"
              onSubmit={async (event) => {
                event.preventDefault();
                if (await change("api/users", "POST", { username, password })) {
                  setCreating(false);
                  setPassword("");
                }
              }}
            >
              <label>
                Username
                <input
                  required
                  maxLength={128}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </label>
              <label>
                Local password (optional)
                <input
                  type="password"
                  minLength={12}
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                />
              </label>
              <p className="user-note">
                Without a password, link an external identity to allow sign-in.
              </p>
              <button className="primary">Create user</button>
            </form>
          ) : (
            selected && (
              <>
                <div className="detail-section user-form">
                  <h3>Account</h3>
                  {!selected.identities.some(
                    (identity) => identity.provider === "proxy",
                  ) && (
                    <form
                      className="user-form"
                      onSubmit={async (event) => {
                        event.preventDefault();
                        await change(
                          `${accountPath(selected.username)}/username`,
                          "PUT",
                          { username },
                          selected.username === currentUser,
                        );
                      }}
                    >
                      <label>
                        Username
                        <input
                          required
                          maxLength={128}
                          value={username}
                          onChange={(event) => setUsername(event.target.value)}
                        />
                      </label>
                      <button className="secondary">Change username</button>
                    </form>
                  )}
                  <form
                    className="user-form"
                    onSubmit={async (event) => {
                      event.preventDefault();
                      await change(
                        `${accountPath(selected.username)}/profile`,
                        "PUT",
                        { role, email, displayName },
                      );
                    }}
                  >
                    <label>
                      Display name
                      <input
                        maxLength={128}
                        value={displayName}
                        onChange={(event) => setDisplayName(event.target.value)}
                      />
                    </label>
                    <label>
                      Email
                      <input
                        type="email"
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                      />
                    </label>
                    <label>
                      Role
                      <select
                        value={role}
                        disabled={selected.username === currentUser}
                        onChange={(event) => setRole(event.target.value)}
                      >
                        <option value="admin">Administrator</option>
                        <option value="editor">Editor</option>
                        <option value="viewer">Viewer</option>
                      </select>
                    </label>
                    <button className="secondary">Save profile</button>
                  </form>
                  {selected.identities.length > 0 &&
                    selected.emailOverridden && (
                      <button
                        className="secondary"
                        onClick={async () => {
                          if (
                            await change(
                              `${accountPath(selected.username)}/profile/revert`,
                              "POST",
                              { email: true, displayName: false },
                            )
                          ) {
                            const refreshed =
                              await request<Account[]>("api/users");
                            const updated = refreshed.find(
                              (user) => user.username === selected.username,
                            );
                            if (updated) setEmail(updated.email);
                          }
                        }}
                      >
                        Restore provider email
                      </button>
                    )}
                  {selected.identities.length > 0 &&
                    selected.displayNameOverridden && (
                      <button
                        className="secondary"
                        onClick={async () => {
                          if (
                            await change(
                              `${accountPath(selected.username)}/profile/revert`,
                              "POST",
                              { email: false, displayName: true },
                            )
                          ) {
                            const refreshed =
                              await request<Account[]>("api/users");
                            const updated = refreshed.find(
                              (user) => user.username === selected.username,
                            );
                            if (updated) setDisplayName(updated.displayName);
                          }
                        }}
                      >
                        Restore provider display name
                      </button>
                    )}
                  <form
                    className="user-form"
                    onSubmit={async (event) => {
                      event.preventDefault();
                      await change(
                        `${accountPath(selected.username)}/groups`,
                        "PUT",
                        { groups: memberGroups },
                      );
                    }}
                  >
                    <h3>Groups</h3>
                    {groups.length === 0 && (
                      <p className="user-note">No groups configured.</p>
                    )}
                    {groups.map((group) => (
                      <label className="user-toggle" key={group}>
                        <input
                          type="checkbox"
                          checked={memberGroups.includes(group)}
                          onChange={(event) =>
                            setMemberGroups(
                              event.target.checked
                                ? [...memberGroups, group]
                                : memberGroups.filter((item) => item !== group),
                            )
                          }
                        />
                        {group}
                      </label>
                    ))}
                    {selected.externalGroups.length > 0 && (
                      <p className="user-note">
                        OIDC memberships: {selected.externalGroups.join(", ")}
                      </p>
                    )}
                    <button className="secondary">Save groups</button>
                  </form>
                  <label className="user-toggle">
                    <input
                      type="checkbox"
                      checked={selected.enabled}
                      disabled={selected.username === currentUser}
                      onChange={(event) =>
                        void change(accountPath(selected.username), "PUT", {
                          enabled: event.target.checked,
                          password: "",
                        })
                      }
                    />{" "}
                    Account enabled
                  </label>
                  <label className="user-toggle">
                    <input
                      type="checkbox"
                      checked={selected.localCredentialEnabled}
                      disabled={selected.username === currentUser}
                      onChange={(event) =>
                        void change(
                          `${accountPath(selected.username)}/credential`,
                          "PUT",
                          { enabled: event.target.checked },
                        )
                      }
                    />{" "}
                    Local password enabled
                  </label>
                  <button
                    className="secondary"
                    onClick={async () => {
                      if (
                        await confirm(
                          "Sign out all browser sessions for this user? Trusted proxy authentication may sign them in again.",
                        )
                      )
                        void change(
                          `${accountPath(selected.username)}/sessions/revoke`,
                          "POST",
                          {},
                          selected.username === currentUser,
                        );
                    }}
                  >
                    Sign out all sessions
                  </button>
                  <form
                    className="user-form"
                    onSubmit={async (event) => {
                      event.preventDefault();
                      if (
                        await change(
                          accountPath(selected.username),
                          "PUT",
                          { enabled: selected.enabled, password },
                          selected.username === currentUser,
                        )
                      )
                        setPassword("");
                    }}
                  >
                    <label>
                      Set local password
                      <input
                        type="password"
                        required
                        minLength={12}
                        maxLength={72}
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        autoComplete="new-password"
                      />
                    </label>
                    <button className="secondary">Update password</button>
                  </form>
                </div>
                <div className="detail-section user-form">
                  <h3>External identities</h3>
                  {selected.identities.length === 0 && (
                    <p className="user-note">No identities linked.</p>
                  )}
                  {selected.identities.map((identity) => (
                    <div
                      className="grant"
                      key={`${identity.provider}:${identity.issuer}:${identity.subject}`}
                    >
                      <div>
                        <strong>{identity.provider}</strong>
                        <small>
                          {identity.issuer && `${identity.issuer} · `}
                          {identity.subject}
                        </small>
                      </div>
                      {identity.provider === "proxy" && (
                        <button
                          className="secondary"
                          onClick={async () => {
                            const newSubject = await prompt(
                              "New trusted-proxy username",
                              identity.subject,
                            );
                            if (newSubject && newSubject !== identity.subject)
                              void change(
                                `${accountPath(selected.username)}/identities/proxy/relink`,
                                "POST",
                                { oldSubject: identity.subject, newSubject },
                              );
                          }}
                        >
                          Relink
                        </button>
                      )}
                      <button
                        className="icon-button danger"
                        title="Unlink identity"
                        onClick={async () => {
                          if (
                            await confirm(
                              "Unlink this identity? Existing sessions will be revoked.",
                            )
                          )
                            void change(
                              `${accountPath(selected.username)}/identities`,
                              "DELETE",
                              identity,
                            );
                        }}
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  ))}
                  <form
                    className="user-form"
                    onSubmit={async (event) => {
                      event.preventDefault();
                      if (
                        await change(
                          `${accountPath(selected.username)}/identities`,
                          "POST",
                          {
                            provider,
                            issuer: provider === "oidc" ? issuer : "",
                            subject,
                          },
                        )
                      ) {
                        setSubject("");
                        setIssuer("");
                      }
                    }}
                  >
                    <label>
                      Provider
                      <select
                        value={provider}
                        onChange={(event) => setProvider(event.target.value)}
                      >
                        <option value="proxy">Trusted proxy</option>
                        <option value="oidc">OIDC</option>
                      </select>
                    </label>
                    {provider === "oidc" && (
                      <label>
                        OIDC issuer
                        <input
                          required
                          type="url"
                          value={issuer}
                          onChange={(event) => setIssuer(event.target.value)}
                        />
                      </label>
                    )}
                    <label>
                      {provider === "oidc" ? "OIDC subject" : "Proxy username"}
                      <input
                        required
                        value={subject}
                        onChange={(event) => setSubject(event.target.value)}
                      />
                    </label>
                    <button className="secondary">
                      <Plus size={15} /> Link identity
                    </button>
                  </form>
                </div>
                {selected.username !== currentUser && (
                  <button
                    className="delete-bucket"
                    onClick={async () => {
                      if (
                        await confirm(`Delete account “${selected.username}”?`)
                      ) {
                        if (
                          await change(
                            accountPath(selected.username),
                            "DELETE",
                            {},
                          )
                        )
                          setSelected(undefined);
                      }
                    }}
                  >
                    <Trash2 size={16} /> Delete user
                  </button>
                )}
              </>
            )
          )}
        </DialogPanel>
      )}
    </>
  );
}
