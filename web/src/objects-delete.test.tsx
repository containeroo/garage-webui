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

test("selecting a folder offers recursive delete and refreshes on confirmation", async () => {
  const list = vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  const remove = vi.spyOn(s3, "deleteFolder").mockResolvedValue(3);
  const user = userEvent.setup();
  renderObjects();

  await user.click(
    await screen.findByRole("checkbox", { name: "Select folder backups/" }),
  );
  await user.click(screen.getByRole("button", { name: "Delete 1" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/deleted recursively/)).toBeTruthy();
  await user.click(within(dialog).getByRole("button", { name: "Confirm" }));

  await waitFor(() =>
    expect(remove).toHaveBeenCalledWith(
      "bucket",
      "backups/",
      expect.any(Function),
    ),
  );
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  expect(screen.queryByRole("button", { name: "Delete 1" })).toBeNull();
});

test("cancelling a folder delete leaves the folder untouched", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  const remove = vi.spyOn(s3, "deleteFolder").mockResolvedValue(0);
  const user = userEvent.setup();
  renderObjects();

  await user.click(
    await screen.findByRole("checkbox", { name: "Select folder backups/" }),
  );
  await user.click(screen.getByRole("button", { name: "Delete 1" }));
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );
  expect(remove).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Delete 1" })).toBeTruthy();
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
  expect(screen.queryByRole("button", { name: /Delete/ })).toBeNull();
});

test("partial folder-deletion failures stay visible after the list refreshes", async () => {
  vi.spyOn(s3, "listObjects").mockResolvedValue({
    folders: ["backups/"],
    items: [],
    nextToken: "",
  });
  vi.spyOn(s3, "deleteFolder").mockRejectedValue(
    new Error("Garage refused delete (403)"),
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
    "Garage refused delete (403)",
  );
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Delete 1" })).toBeNull(),
  );
  expect(screen.getByRole("alert").textContent).toContain(
    "Garage refused delete (403)",
  );
});
