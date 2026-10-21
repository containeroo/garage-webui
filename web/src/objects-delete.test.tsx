// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BucketObjects } from "./components/objects";
import { QuestionProvider } from "./components/ui/questions";
import * as s3 from "./s3";
import * as jobs from "./jobs";

/** renderObjects mounts the authenticated object browser with confirmation dialogs. */
function renderObjects(canDelete = true) {
  return render(
    <QuestionProvider>
      <BucketObjects
        bucketId="bucket"
        canDelete={canDelete}
        enabled
        refreshVersion={0}
      />
    </QuestionProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("folder delete queues backend job without deleting in the browser", async () => {
  const list = vi
    .spyOn(s3, "listObjects")
    .mockResolvedValue({ folders: ["backups/"], items: [], nextToken: "" });
  const create = vi
    .spyOn(jobs, "startDeletion")
    .mockResolvedValue({
      id: "a",
      bucket: "bucket",
      status: "pending",
      deleted: 0,
      cancelRequested: false,
      createdAt: "",
      updatedAt: "",
    });
  const user = userEvent.setup();
  renderObjects();
  await user.click(
    await screen.findByRole("checkbox", { name: "Select folder backups/" }),
  );
  await user.click(screen.getByRole("button", { name: "Delete 1" }));
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: "Confirm",
    }),
  );
  await waitFor(() =>
    expect(create).toHaveBeenCalledWith("bucket", [], ["backups/"]),
  );
  expect(await screen.findByText(/Deletion queued/)).toBeTruthy();
  expect(list).toHaveBeenCalled();
});

test("cancelling confirmation never queues a deletion", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  const create = vi.spyOn(jobs, "startDeletion");
  const user = userEvent.setup();
  renderObjects();
  await user.click(
    await screen.findByRole("checkbox", { name: "Select folder backups/" }),
  );
  await user.click(screen.getByRole("button", { name: "Delete 1" }));
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  expect(create).not.toHaveBeenCalled();
});

test("read-only browsing never offers folder deletion", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  renderObjects(false);
  await screen.findByRole("button", { name: /backups\// });
  expect(
    screen.queryByRole("checkbox", { name: "Select folder backups/" }),
  ).toBeNull();
});

test("failed scheduling presents the error without deleting objects", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  vi.spyOn(jobs, "startDeletion").mockRejectedValue(
    new Error("cannot queue deletion"),
  );
  const user = userEvent.setup();
  renderObjects();
  await user.click(
    await screen.findByRole("checkbox", { name: "Select folder backups/" }),
  );
  await user.click(screen.getByRole("button", { name: "Delete 1" }));
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Confirm" }),
  );
  expect((await screen.findByRole("alert")).textContent).toContain(
    "cannot queue deletion",
  );
});
