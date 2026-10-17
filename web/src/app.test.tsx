// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./app";
import { BucketObjects } from "./components/objects";
import { QuestionProvider } from "./components/ui/questions";
import * as s3 from "./s3";

const session = (user: string) => ({
  mode: "local",
  user,
  role: "admin",
  setupRequired: false,
  localLogin: true,
  uploadsEnabled: false,
});
const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.history.replaceState(null, "", "/");
});

test("shows selected object details and clears them when navigating folders", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [
      { key: "report.txt", size: 1200, modified: "2026-01-01T00:00:00Z" },
    ],
    nextToken: "",
  });
  const user = userEvent.setup();
  render(
    <QuestionProvider>
      <BucketObjects bucketId="bucket" canDelete enabled refreshVersion={0} />
    </QuestionProvider>,
  );
  await user.click(await screen.findByRole("button", { name: "report.txt" }));
  const details = screen.getByRole("complementary", { name: "Object details" });
  expect(within(details).getAllByText("report.txt").length).toBeGreaterThan(0);
  expect(within(details).getByText(/1,200 bytes/)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: /backups\// }));
  expect(within(details).getByText("Details")).toBeTruthy();
});

test("allows retrying a failed initial session request", async () => {
  let calls = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      if (++calls === 1) throw new Error("offline");
      return json(session(""));
    }),
  );
  render(<App />);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Could not connect",
  );
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  expect(await screen.findByRole("heading", { name: "Sign in" })).toBeTruthy();
});

test("overview lists each bucket once with its usage and links to all buckets", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/api/session")) return json(session("alice"));
      if (path.includes("ListBuckets"))
        return json([
          { id: "bucket-1", globalAliases: ["archive"], created: "" },
        ]);
      if (path.includes("GetBucketInfo"))
        return json({
          id: "bucket-1",
          globalAliases: ["archive"],
          bytes: 1200,
          objects: 2,
        });
      if (path.includes("ListKeys")) return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  expect(await screen.findAllByText("archive")).toHaveLength(1);
  expect(
    within(screen.getByRole("button", { name: /archive/ })).getByText("1.2 kB"),
  ).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "View all" }));
  expect(await screen.findByRole("heading", { name: "Buckets" })).toBeTruthy();
  for (const [action, tab] of [
    ["Overview", "Overview"],
    ["Permissions", "Permissions"],
    ["Manage objects", "Browse"],
  ]) {
    await userEvent.click(
      screen.getByRole("button", { name: `${action}: archive` }),
    );
    expect(
      (await screen.findByRole("tab", { name: tab })).getAttribute(
        "aria-selected",
      ),
    ).toBe("true");
    await userEvent.click(
      screen.getByRole("button", { name: /Back to buckets/ }),
    );
  }
});

test("keeps action errors visible inside the open dialog", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/api/session")) return json(session("alice"));
      if (path.includes("CreateBucket"))
        return json({ error: "Bucket already exists" }, 409);
      if (path.includes("ListBuckets") || path.includes("ListKeys"))
        return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Buckets" }));
  await user.click(screen.getByRole("button", { name: /Create bucket/i }));
  const dialog = await screen.findByRole("dialog");
  await user.type(within(dialog).getByLabelText("Name"), "existing");
  await user.click(within(dialog).getByRole("button", { name: "Create" }));
  expect((await within(dialog).findByRole("alert")).textContent).toContain(
    "Bucket already exists",
  );
});

test("late responses from a signed-out account cannot populate the next workspace", async () => {
  let currentUser = "alice";
  let releaseOldBuckets!: (response: Response) => void;
  let oldBucketsRequested!: () => void;
  const oldRequestStarted = new Promise<void>((resolve) => {
    oldBucketsRequested = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/api/session")) return json(session(currentUser));
      if (path.endsWith("/api/logout"))
        return new Response(null, { status: 204 });
      if (path.endsWith("/api/login")) {
        currentUser = "bob";
        return json({ user: "bob" });
      }
      if (path.includes("ListBuckets") && currentUser === "alice") {
        oldBucketsRequested();
        return new Promise<Response>((resolve) => {
          releaseOldBuckets = resolve;
        });
      }
      if (path.includes("ListBuckets") || path.includes("ListKeys"))
        return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await screen.findByRole("button", { name: "Sign out" });
  await oldRequestStarted;
  await user.click(screen.getByRole("button", { name: "Sign out" }));
  await user.type(await screen.findByLabelText("Username"), "bob");
  await user.type(screen.getByLabelText("Password"), "password");
  await user.click(screen.getByRole("button", { name: "Sign in" }));
  await screen.findByText("bob");
  await act(async () => {
    releaseOldBuckets(
      json([{ id: "old", globalAliases: ["alice-secret"], created: "" }]),
    );
    await Promise.resolve();
  });
  await user.click(screen.getByRole("button", { name: "Buckets" }));
  expect(screen.queryByText("alice-secret")).toBeNull();
});

test("clears loaded data when signing in as another account", async () => {
  let currentUser = "alice";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/api/session")) return json(session(currentUser));
      if (path.endsWith("/api/logout"))
        return new Response(null, { status: 204 });
      if (path.endsWith("/api/login")) {
        currentUser = "bob";
        return json({ user: "bob" });
      }
      if (path.includes("ListBuckets"))
        return currentUser === "alice"
          ? json([{ id: "old", globalAliases: ["alice-secret"], created: "" }])
          : json({ error: "Storage unavailable" }, 503);
      if (path.includes("ListKeys")) return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Buckets" }));
  expect(await screen.findByText("alice-secret")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Sign out" }));
  await user.type(await screen.findByLabelText("Username"), "bob");
  await user.type(screen.getByLabelText("Password"), "password");
  await user.click(screen.getByRole("button", { name: "Sign in" }));
  await screen.findByText("bob");
  await user.click(screen.getByRole("button", { name: "Buckets" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Storage unavailable",
  );
  expect(screen.queryByText("alice-secret")).toBeNull();
});

test("user management uses accessible modals and keeps errors inside them", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const path = String(input);
      if (path.endsWith("/api/session")) return json(session("alice"));
      if (path.endsWith("/api/users") && options?.method === "POST")
        return json({ error: "Account already exists" }, 409);
      if (path.endsWith("/api/users"))
        return json([
          {
            username: "alice",
            role: "admin",
            email: "",
            displayName: "",
            groups: [],
            externalGroups: [],
            identities: [],
            enabled: true,
            hasPassword: true,
            localCredentialEnabled: true,
          },
        ]);
      if (path.endsWith("/api/registration")) return json({ enabled: false });
      if (
        path.endsWith("/api/pending-identities") ||
        path.endsWith("/api/groups") ||
        path.endsWith("/api/group-mappings") ||
        path.includes("ListBuckets") ||
        path.includes("ListKeys")
      )
        return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Users" }));
  const create = await screen.findByRole("button", { name: "Create user" });
  await user.click(create);
  const dialog = screen.getByRole("dialog", { name: "Create user" });
  await user.type(within(dialog).getByLabelText("Username"), "existing");
  await user.click(within(dialog).getByRole("button", { name: "Create user" }));
  expect((await within(dialog).findByRole("alert")).textContent).toContain(
    "Account already exists",
  );
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(create);
  const manage = screen.getByRole("button", { name: "Manage" });
  await user.click(manage);
  expect(screen.getByRole("dialog", { name: "Manage alice" })).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Close dialog" }));
  expect(document.activeElement).toBe(manage);
});

test("custom confirmation waits for a choice and cancellation does not run the action", async () => {
  const { QuestionProvider, useQuestions } =
    await import("./components/ui/questions");
  const action = vi.fn();
  function Example() {
    const { confirm } = useQuestions();
    return (
      <button
        onClick={async () => {
          if (await confirm("Discard changes?")) action();
        }}
      >
        Discard
      </button>
    );
  }
  render(
    <QuestionProvider>
      <Example />
    </QuestionProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Discard" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("Discard changes?")).toBeTruthy();
  expect(action).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(action).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Discard" }));
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Confirm" }),
  );
  expect(action).toHaveBeenCalledOnce();
});

test("stages a node assignment with the chosen capacity unit", async () => {
  const calls: { path: string; body?: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      calls.push({ path, body: init?.body as string | undefined });
      if (path.endsWith("/api/session")) return json(session("alice"));
      if (path.includes("ListBuckets") || path.includes("ListKeys"))
        return json([]);
      if (path.includes("GetClusterStatus"))
        return json({
          layoutVersion: 1,
          nodes: [
            {
              id: "node1",
              hostname: "node one",
              isUp: true,
              draining: false,
              role: { zone: "z1", capacity: 1000000000, tags: [] },
            },
          ],
        });
      if (path.includes("GetClusterLayout"))
        return json({
          version: 1,
          roles: [{ id: "node1", zone: "z1", capacity: 1000000000, tags: [] }],
          parameters: { zoneRedundancy: "maximum" },
          stagedRoleChanges: [],
        });
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Cluster" }));
  await user.click(await screen.findByRole("button", { name: "Assign" }));
  const dialog = screen.getByRole("dialog", { name: "Assign node" });
  await user.clear(within(dialog).getByLabelText("Capacity"));
  await user.type(within(dialog).getByLabelText("Capacity"), "2");
  await user.selectOptions(
    within(dialog).getByLabelText("Capacity unit"),
    "TB",
  );
  await user.click(
    within(dialog).getByRole("button", { name: "Stage assignment" }),
  );
  await vi.waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.path.includes("UpdateClusterLayout") &&
          JSON.parse(c.body || "{}").roles?.[0]?.capacity === 2000000000000,
      ),
    ).toBe(true),
  );
});

test("loads and saves the account theme", async () => {
  const calls: { path: string; body?: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      calls.push({ path, body: init?.body as string | undefined });
      if (path.endsWith("/api/session")) return json(session("alice"));
      if (path.endsWith("/api/themes"))
        return json([
          {
            title: "Light",
            color_scheme: "light",
            colors: { background: "#ffffff" },
          },
          {
            title: "Dark",
            color_scheme: "dark",
            colors: { background: "#111318" },
          },
          {
            title: "Nord",
            color_scheme: "dark",
            colors: { background: "#2e3440" },
          },
        ]);
      if (path.endsWith("/api/profile"))
        return json({
          username: "alice",
          email: "",
          displayName: "",
          localCredentialEnabled: false,
          theme: "Dark",
        });
      if (path.endsWith("/api/profile/theme")) return json({});
      if (path.includes("ListBuckets") || path.includes("ListKeys"))
        return json([]);
      return json({ status: "healthy" });
    }),
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /alice admin/i }));
  const select = await screen.findByLabelText("Theme");
  await vi.waitFor(() =>
    expect((select as HTMLSelectElement).value).toBe("Dark"),
  );
  expect(document.documentElement.dataset.theme).toBe("dark");
  await user.selectOptions(select, "Nord");
  await vi.waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.path.endsWith("/api/profile/theme") &&
          c.body === JSON.stringify({ theme: "Nord" }),
      ),
    ).toBe(true),
  );
  await vi.waitFor(() =>
    expect(
      document.documentElement.style.getPropertyValue("--theme-background"),
    ).toBeTruthy(),
  );
});
