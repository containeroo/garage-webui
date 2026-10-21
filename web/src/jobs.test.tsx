// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BackgroundJobs } from "./components/jobs";
import * as jobs from "./jobs";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

test("background operations reload only when the user requests it", async () => {
  const list = vi.spyOn(jobs, "listDeletions").mockResolvedValue([
    {
      id: "job-1",
      bucket: "backups",
      target: "daily/",
      status: "running",
      deleted: 100,
      cancelRequested: false,
      createdAt: "2026-10-21T12:00:00Z",
      updatedAt: "2026-10-21T12:00:00Z",
    },
  ]);
  const user = userEvent.setup();

  render(<BackgroundJobs />);

  expect(await screen.findByText(/daily\//)).toBeTruthy();
  expect(list).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole("button", { name: "Refresh status" }));
  await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
});
