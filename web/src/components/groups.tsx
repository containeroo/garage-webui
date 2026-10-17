import { useQuestions } from "./ui/questions";
import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";

// GroupManagement manages local groups and OIDC mappings.
export function GroupManagement({
  groups,
  mappings,
  change,
}: {
  groups: string[];
  mappings: { externalGroup: string; group: string }[];
  change: (path: string, method: string, body: unknown) => Promise<boolean>;
}) {
  const { confirm } = useQuestions();
  const [groupName, setGroupName] = useState("");
  const [externalGroup, setExternalGroup] = useState("");
  const [mappedGroup, setMappedGroup] = useState("");
  return (
    <>
      <section className="panel user-list">
        <div className="panel-head">
          <div>
            <h2>Groups</h2>
          </div>
        </div>
        <form
          className="user-row"
          onSubmit={async (event) => {
            event.preventDefault();
            if (await change("api/groups", "POST", { name: groupName }))
              setGroupName("");
          }}
        >
          <input
            aria-label="Group name"
            required
            maxLength={128}
            value={groupName}
            onChange={(event) => setGroupName(event.target.value)}
          />
          <button className="secondary">
            <Plus size={15} /> Create group
          </button>
        </form>
        {groups.map((group) => (
          <div className="user-row" key={group}>
            <div>
              <strong>{group}</strong>
            </div>
            <button
              className="icon-button danger"
              title={`Delete ${group}`}
              onClick={async () => {
                if (
                  await confirm(`Delete group “${group}” and its memberships?`)
                )
                  void change(
                    `api/groups/${encodeURIComponent(group)}`,
                    "DELETE",
                    {},
                  );
              }}
            >
              <Trash2 size={16} />
            </button>
          </div>
        ))}
      </section>
      <section className="panel user-list">
        <div className="panel-head">
          <div>
            <h2>OIDC group mappings</h2>
          </div>
        </div>
        <form
          className="user-row"
          onSubmit={async (event) => {
            event.preventDefault();
            if (
              await change("api/group-mappings", "POST", {
                externalGroup,
                group: mappedGroup,
              })
            )
              setExternalGroup("");
          }}
        >
          <input
            aria-label="External group"
            required
            maxLength={128}
            value={externalGroup}
            onChange={(event) => setExternalGroup(event.target.value)}
          />
          <select
            aria-label="Local group"
            required
            value={mappedGroup}
            onChange={(event) => setMappedGroup(event.target.value)}
          >
            <option value="">Choose group</option>
            {groups.map((group) => (
              <option key={group} value={group}>
                {group}
              </option>
            ))}
          </select>
          <button className="secondary">
            <Plus size={15} /> Add mapping
          </button>
        </form>
        {mappings.map((mapping) => (
          <div className="user-row" key={JSON.stringify(mapping)}>
            <div>
              <strong>
                {mapping.externalGroup} → {mapping.group}
              </strong>
            </div>
            <button
              className="icon-button danger"
              title="Remove mapping"
              onClick={async () => {
                if (await confirm("Remove this group mapping?"))
                  void change("api/group-mappings", "DELETE", mapping);
              }}
            >
              <Trash2 size={16} />
            </button>
          </div>
        ))}
      </section>
    </>
  );
}
