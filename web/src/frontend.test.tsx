// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
} from "@testing-library/react";
import { request, type Session } from "./api";
import { localDateTime, formatBytes, formatBinaryBytes } from "./lib/format";
import { uploadObject } from "./s3";
import * as s3 from "./s3";
import { useWorkspaceData } from "./hooks/use-workspace-data";
import { useBucketUsage } from "./hooks/use-bucket-usage";
import { QuestionProvider, useQuestions } from "./components/ui/questions";
import { Cluster } from "./components/cluster";
import { BucketSettings } from "./components/bucket-settings";
import { BucketObjects } from "./components/objects";
import { UploadDialog } from "./components/upload";
import { Button } from "./components/ui/button";

const session: Session = {
  mode: "admin",
  user: "alice",
  scope: ["*"],
  uploadsEnabled: true,
};
const json = (value: unknown) =>
  new Response(JSON.stringify(value), {
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("shared buttons use the theme-aware action styles", () => {
  render(
    <>
      <Button disabled>Confirm</Button>
      <Button variant="outline">Cancel</Button>
      <Button asChild>
        <a href="#details">View details</a>
      </Button>
    </>,
  );

  const confirm = screen.getByRole("button", { name: "Confirm" });
  expect(confirm.classList.contains("primary")).toBe(true);
  expect(confirm.hasAttribute("disabled")).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Cancel" })
      .classList.contains("secondary"),
  ).toBe(true);
  expect(
    screen
      .getByRole("link", { name: "View details" })
      .classList.contains("primary"),
  ).toBe(true);
});

test("bounds usage requests and only publishes complete aggregate totals", async () => {
  const pending: ((response: Response) => void)[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>((resolve) => pending.push(resolve))),
  );
  const buckets = Array.from({ length: 8 }, (_, index) => ({
    id: String(index),
    globalAliases: [],
    created: "",
  }));
  const totals = vi.fn();
  const hook = renderHook(() => useBucketUsage(buckets, true, false, totals));
  expect(pending).toHaveLength(6);
  await act(async () => {
    pending
      .slice(0, 6)
      .forEach((resolve, index) =>
        resolve(json({ id: String(index), bytes: 10, objects: 1 })),
      );
  });
  expect(pending).toHaveLength(8);
  expect(totals).not.toHaveBeenCalledWith({ bytes: 60, objects: 6 });
  await act(async () => {
    pending
      .slice(6)
      .forEach((resolve, index) =>
        resolve(json({ id: String(index + 6), bytes: 10, objects: 1 })),
      );
  });
  expect(hook.result.current.data).toHaveLength(8);
  expect(totals).toHaveBeenLastCalledWith({ bytes: 80, objects: 8 });
});

test("preserves Headers objects, tuple headers, and caller content types", async () => {
  const fetch = vi.fn(async () => json({}));
  vi.stubGlobal("fetch", fetch);
  for (const headers of [
    new Headers({
      "X-Test": "kept",
      "Content-Type": "application/problem+json",
    }),
    [
      ["X-Test", "kept"],
      ["Content-Type", "application/problem+json"],
    ] as [string, string][],
  ]) {
    await request("api/session", { headers });
    const sent = fetch.mock.calls.at(-1) as unknown as [URL, RequestInit];
    const actual = new Headers(sent[1].headers);
    expect(actual.get("X-Test")).toBe("kept");
    expect(actual.get("Content-Type")).toBe("application/problem+json");
  }
});

test("formats storage units explicitly and rejects unavailable quantities", () => {
  expect(formatBytes(1200)).toBe("1.2 kB");
  expect(formatBinaryBytes(1024)).toBe("1.0 KiB");
  for (const invalid of [-1, NaN, Infinity])
    expect(formatBytes(invalid)).toBe("—");
});

test("converts UTC expiration to the local input time without changing its instant", () => {
  vi.spyOn(Date.prototype, "getTimezoneOffset").mockReturnValue(-120);
  expect(localDateTime("2026-07-01T12:00:00Z")).toBe("2026-07-01T14:00");
  expect(localDateTime("invalid")).toBe("");
});

test("rejects an upload cancelled before transfer even if signing ignores cancellation", async () => {
  const controller = new AbortController();
  controller.abort();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      json({ url: "https://s3.example/object", method: "PUT" }),
    ),
  );
  await expect(
    uploadObject(
      "bucket",
      "file.txt",
      new File(["data"], "file.txt"),
      controller.signal,
      vi.fn(),
    ),
  ).rejects.toMatchObject({ name: "AbortError" });
});

test("declines an outstanding confirmation when its provider unmounts", async () => {
  const hook = renderHook(() => useQuestions(), { wrapper: QuestionProvider });
  let pending!: Promise<boolean>;
  act(() => {
    pending = hook.result.current.confirm("Delete?");
  });
  hook.unmount();
  await expect(pending).resolves.toBe(false);
});

test("does not let an earlier workspace refresh replace a later result", async () => {
  const buckets: ((response: Response) => void)[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: URL) => {
      if (String(url).includes("ListBuckets"))
        return new Promise<Response>((resolve) => buckets.push(resolve));
      if (String(url).includes("ListKeys")) return json([]);
      return json({ status: "healthy" });
    }),
  );
  const hook = renderHook(() => useWorkspaceData(session));
  let newer!: Promise<void>;
  act(() => {
    newer = hook.result.current.load();
  });
  await act(async () => {
    buckets[1](json([{ id: "new", globalAliases: ["new"] }]));
    await newer;
  });
  await act(async () => {
    buckets[0](json([{ id: "old", globalAliases: ["old"] }]));
  });
  expect(hook.result.current.buckets[0].id).toBe("new");
});

test("loads cluster status without requesting a layout outside the token's scopes", async () => {
  const fetch = vi.fn(async (_url: RequestInfo | URL) =>
    json({
      layoutVersion: 1,
      nodes: [
        { id: "node", hostname: "storage-one", isUp: true, draining: false },
      ],
    }),
  );
  vi.stubGlobal("fetch", fetch);
  render(
    <QuestionProvider>
      <Cluster session={{ ...session, scope: ["GetClusterStatus"] }} />
    </QuestionProvider>,
  );
  expect(await screen.findByText("storage-one")).toBeTruthy();
  expect(
    fetch.mock.calls.every(
      (call) => !String(call[0]).includes("GetClusterLayout"),
    ),
  ).toBe(true);
});

test("offers only bucket settings actions allowed by their individual scopes", () => {
  render(
    <QuestionProvider>
      <BucketSettings
        session={{ ...session, scope: ["AddBucketAlias"] }}
        bucket={{ id: "bucket", globalAliases: ["archive"], keys: [] }}
        onChanged={vi.fn()}
      />
    </QuestionProvider>,
  );
  expect(screen.getByRole("button", { name: "Add" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Save limits" })).toBeNull();
  expect(screen.queryByTitle("Remove alias")).toBeNull();
});

test("cancels an object's preview when navigating away from its folder", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["next/"],
    items: [{ key: "photo.png", size: 8, modified: "" }],
    nextToken: "",
  });
  let resolve!: (operation: s3.SignedOperation) => void;
  const signing = vi.spyOn(s3, "signOperation").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  render(
    <QuestionProvider>
      <BucketObjects bucketId="bucket" canDelete enabled refreshVersion={0} />
    </QuestionProvider>,
  );
  fireEvent.click(await screen.findByTitle("Preview"));
  const signal = signing.mock.calls[0][3];
  fireEvent.click(screen.getByRole("button", { name: /^next\// }));
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    resolve({
      url: "https://s3.example/photo.png",
      method: "GET",
      expiresIn: 60,
    });
  });
  expect(screen.queryByRole("img", { name: "photo.png" })).toBeNull();
});

test("ignores dropped files while an upload owns the queue", async () => {
  let finish!: () => void;
  const transfer = vi.spyOn(s3, "uploadObject").mockImplementation(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  const uploaded = vi.fn();
  const view = render(
    <UploadDialog
      bucketId="bucket"
      bucketName="archive"
      onClose={vi.fn()}
      onUploaded={uploaded}
    />,
  );
  const input =
    view.container.querySelector('input[type="file"]') ||
    document.querySelector('input[type="file"]');
  fireEvent.change(input!, {
    target: { files: [new File(["a"], "first.txt")] },
  });
  fireEvent.click(screen.getByRole("button", { name: "Upload file" }));
  fireEvent.drop(document.querySelector(".upload-dropzone")!, {
    dataTransfer: { files: [new File(["b"], "second.txt")] },
  });
  await act(async () => {
    finish();
  });
  expect(screen.queryByText("second.txt")).toBeNull();
  expect(transfer).toHaveBeenCalledOnce();
  expect(uploaded).toHaveBeenCalledOnce();
});
