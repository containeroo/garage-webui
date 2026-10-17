import { test, expect } from "@playwright/test";
import { mkdtemp, open, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const password = "end-to-end-password";
const origin = new URL(process.env.E2E_BASE_URL).origin;
const headers = { Origin: origin, "Content-Type": "application/json" };
const bucket = `e2e-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
let administratorCookies;

async function login(page, username) {
  await page.goto("./");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}

test.describe.serial("local authentication and storage", () => {
  test("first setup creates an administrator and rejects a second setup", async ({
    page,
  }) => {
    await page.goto("./");
    await expect(
      page.getByRole("heading", { name: "Create administrator" }),
    ).toBeVisible();
    await page.getByLabel("Username").fill("e2e-admin");
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByLabel("Confirm password").fill(password);
    await page.getByRole("button", { name: "Create administrator" }).click();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
    administratorCookies = await page.context().cookies();
    const setup = await page.request.post("api/setup", {
      headers,
      data: { username: "second", password },
    });
    expect(setup.status()).toBe(409);
    const create = await page.request.post("api/users", {
      headers,
      data: { username: "e2e-viewer", password },
    });
    expect(create.status()).toBe(204);
    const role = await page.request.put("api/users/e2e-viewer/profile", {
      headers,
      data: { role: "viewer", email: "", displayName: "" },
    });
    expect(role.status()).toBe(204);
  });

  test("enforces viewer authorization and revokes the signed-out session", async ({
    page,
  }) => {
    await login(page, "e2e-admin");
    await page.getByRole("button", { name: "Buckets" }).click();
    await page.getByRole("button", { name: "Create bucket" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Name").fill(bucket);
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(page.getByText(bucket).first()).toBeVisible();
    await page.getByRole("button", { name: "Sign out" }).click();
    await page.getByLabel("Username").fill("e2e-viewer");
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByText("e2e-viewer")).toBeVisible();
    await expect(page.getByRole("button", { name: "Users" })).toHaveCount(0);
    expect((await page.request.get("api/users")).status()).toBe(403);
    expect(
      (
        await page.request.post("api/garage/CreateBucket", {
          headers,
          data: { globalAlias: "e2e-forbidden" },
        })
      ).status(),
    ).toBe(403);
    expect(
      (
        await page.request.post("api/objects/presign", {
          headers,
          data: { action: "delete", bucketId: "unused", key: "x" },
        })
      ).status(),
    ).toBe(403);
    await page.getByRole("button", { name: "Sign out" }).click();
    expect((await page.request.get("api/users")).status()).toBe(401);
  });

  test("manages users in an accessible modal", async ({ page }) => {
    await page.context().addCookies(administratorCookies);
    await page.goto("./");
    await page.getByRole("button", { name: "Users" }).click();
    const create = page.getByRole("button", { name: "Create user" });
    await create.click();
    const dialog = page.getByRole("dialog", { name: "Create user" });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("Username").fill("e2e-viewer");
    await dialog.getByRole("button", { name: "Create user" }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(create).toBeFocused();
    const row = page.locator(".user-row").filter({ hasText: "e2e-viewer" });
    const manage = row.getByRole("button", { name: "Manage" });
    await manage.click();
    const management = page.getByRole("dialog", { name: "Manage e2e-viewer" });
    await expect(
      management.getByRole("heading", { name: "Account" }),
    ).toBeVisible();
    await management.getByRole("button", { name: "Close dialog" }).click();
    await expect(manage).toBeFocused();
  });

  test("does not show a delayed response from a signed-out account", async ({
    page,
  }) => {
    let release;
    let started;
    const requestStarted = new Promise((resolve) => {
      started = resolve;
    });
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    await page.route(
      "**/api/garage/ListBuckets",
      async (route) => {
        started();
        await gate;
        await route
          .fulfill({
            json: [
              {
                id: "old",
                globalAliases: ["previous-session-only"],
                created: "",
              },
            ],
          })
          .catch(() => {});
      },
      { times: 1 },
    );
    try {
      await login(page, "e2e-admin");
      await requestStarted;
      await page.getByRole("button", { name: "Sign out" }).click();
      await page.getByLabel("Username").fill("e2e-viewer");
      await page.getByLabel("Password", { exact: true }).fill(password);
      await page.getByRole("button", { name: "Sign in" }).click();
      await expect(page.getByText("e2e-viewer")).toBeVisible();
      release();
      await page.getByRole("button", { name: "Buckets" }).click();
      await expect(page.getByText(bucket).first()).toBeVisible();
      await expect(page.getByText("previous-session-only")).toHaveCount(0);
    } finally {
      release();
    }
  });

  test("recovers from a failed session request", async ({ page }) => {
    await page.route("**/api/session", (route) => route.abort(), { times: 1 });
    await page.goto("./");
    await expect(page.getByRole("alert")).toContainText("Could not connect");
    await page.getByRole("button", { name: "Try again" }).click();
    await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  });

  test("allows retrying a failed upload", async ({ page }) => {
    await login(page, "e2e-admin");
    await page.getByRole("button", { name: "Buckets" }).click();
    await page.getByText(bucket).first().click();
    await page.getByRole("tab", { name: "Browse" }).click();
    await page
      .getByRole("button", { name: /upload/i })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.locator('input[type="file"]').setInputFiles({
      name: "retry.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("retry"),
    });
    await page.route(
      "**/retry.txt?*",
      (route) =>
        route.request().method() === "PUT"
          ? route.fulfill({ status: 503, body: "unavailable" })
          : route.continue(),
      { times: 1 },
    );
    await dialog.getByRole("button", { name: "Upload file" }).click();
    await expect(dialog.getByText(/failed/i).first()).toBeVisible();
    await dialog.getByRole("button", { name: "Upload file" }).click();
    await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
  });

  test("uploads, previews, downloads and deletes an object", async ({
    page,
  }) => {
    await login(page, "e2e-admin");
    await page.getByRole("button", { name: "Buckets" }).click();
    await page.getByText(bucket).first().click();
    await page.getByRole("tab", { name: "Browse" }).click();
    await page
      .getByRole("button", { name: /upload/i })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.locator('input[type="file"]').setInputFiles({
      name: "hello.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("hello from playwright"),
    });
    await dialog.getByRole("button", { name: "Upload file" }).click();
    await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    const row = page
      .locator(".explorer-row:not(.explorer-folder)")
      .filter({ hasText: "hello.txt" });
    await expect(row).toBeVisible();
    await row.getByTitle("Preview").click();
    await expect(page.getByText("hello from playwright")).toBeVisible();
    await page.getByRole("button", { name: "Close object details" }).click();
    const downloadPromise = page.waitForEvent("download");
    await row.getByTitle("Download").click();
    expect((await downloadPromise).suggestedFilename()).toBe("hello.txt");
    await row.getByRole("button", { name: "hello.txt" }).click();
    await page.getByRole("button", { name: "Copy", exact: true }).click();
    await page.getByRole("dialog").getByRole("textbox").fill("copy.txt");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Confirm" })
      .click();
    const copy = page
      .locator(".explorer-row:not(.explorer-folder)")
      .filter({ hasText: "copy.txt" });
    await expect(copy).toBeVisible();
    await copy.getByRole("button", { name: "copy.txt" }).click();
    await page.getByRole("button", { name: "Move or rename" }).click();
    await page.getByRole("dialog").getByRole("textbox").fill("renamed.txt");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Confirm" })
      .click();
    const renamed = page
      .locator(".explorer-row:not(.explorer-folder)")
      .filter({ hasText: "renamed.txt" });
    await expect(renamed).toBeVisible();
    await expect(copy).toHaveCount(0);
    for (const item of [row, renamed]) {
      await item.locator(".explorer-name").click();
      await page.getByRole("button", { name: "Delete", exact: true }).click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Confirm" })
        .click();
      await expect(item).toHaveCount(0);
    }
  });

  test("completes a multipart upload", async ({ page }) => {
    test.skip(
      process.env.E2E_LARGE_UPLOAD !== "1",
      "Set E2E_LARGE_UPLOAD=1 to run the 257 MiB multipart test",
    );
    test.setTimeout(300_000);
    const dir = await mkdtemp(join(tmpdir(), "garage-webui-e2e-"));
    const path = join(dir, "multipart.bin");
    try {
      const file = await open(path, "w");
      try {
        await file.truncate(257 * 1024 * 1024);
      } finally {
        await file.close();
      }
      await login(page, "e2e-admin");
      await page.getByRole("button", { name: "Buckets" }).click();
      await page.getByText(bucket).first().click();
      await page.getByRole("tab", { name: "Browse" }).click();
      await page
        .getByRole("button", { name: /upload/i })
        .first()
        .click();
      const dialog = page.getByRole("dialog");
      await dialog.locator('input[type="file"]').setInputFiles(path);
      const complete = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().includes("uploadId="),
      );
      await dialog.getByRole("button", { name: "Upload file" }).click();
      expect((await complete).ok()).toBeTruthy();
      await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
      await dialog.getByRole("button", { name: "Close", exact: true }).click();
      await expect(
        page
          .locator(".explorer-row:not(.explorer-folder)")
          .filter({ hasText: "multipart.bin" }),
      ).toBeVisible();
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  test("cancels and aborts a multipart upload", async ({ page }) => {
    test.skip(
      process.env.E2E_LARGE_UPLOAD !== "1",
      "Set E2E_LARGE_UPLOAD=1 to run the 257 MiB multipart test",
    );
    const dir = await mkdtemp(join(tmpdir(), "garage-webui-e2e-"));
    const path = join(dir, "cancel.bin");
    let release;
    try {
      const file = await open(path, "w");
      try {
        await file.truncate(257 * 1024 * 1024);
      } finally {
        await file.close();
      }
      await login(page, "e2e-admin");
      await page.getByRole("button", { name: "Buckets" }).click();
      await page.getByText(bucket).first().click();
      await page.getByRole("tab", { name: "Browse" }).click();
      await page
        .getByRole("button", { name: /upload/i })
        .first()
        .click();
      const dialog = page.getByRole("dialog");
      await dialog.locator('input[type="file"]').setInputFiles(path);
      let partStarted;
      const part = new Promise((resolve) => {
        partStarted = resolve;
      });
      const gate = new Promise((resolve) => {
        release = resolve;
      });
      await page.route("**/*", async (route) => {
        if (
          route.request().method() === "PUT" &&
          route.request().url().includes("partNumber=")
        ) {
          partStarted();
          await gate;
          await route.abort().catch(() => {});
        } else await route.continue();
      });
      await dialog.getByRole("button", { name: "Upload file" }).click();
      await part;
      const abort = page.waitForResponse(
        (response) =>
          response.request().method() === "DELETE" &&
          response.url().includes("uploadId="),
      );
      await dialog.getByRole("button", { name: "Cancel uploads" }).click();
      release();
      expect((await abort).ok()).toBeTruthy();
      await expect(dialog.getByText("Uploads cancelled")).toBeVisible();
    } finally {
      release?.();
      await rm(dir, { recursive: true, force: true });
    }
  });

  test("navigates object listing pages and resets pagination when filtering", async ({
    page,
  }) => {
    await page.context().addCookies(administratorCookies);
    await page.goto("./");
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
    const listing = await page.request.get("api/garage/ListBuckets");
    expect(listing.ok()).toBeTruthy();
    const buckets = await listing.json();
    const bucketId = buckets.find((item) =>
      item.globalAliases?.includes(bucket),
    )?.id;
    expect(bucketId).toBeTruthy();

    // Seed real S3 objects through the same scoped upload authorization as the UI.
    async function createObject(index) {
      const key = `pagination-${String(index).padStart(3, "0")}.txt`;
      const signed = await page.request.post("api/uploads/presign", {
        headers,
        data: { bucketId, key, size: 1 },
      });
      expect(signed.ok(), `presign ${key}: ${signed.status()}`).toBeTruthy();
      const { url } = await signed.json();
      const upload = await page.request.put(url, { data: Buffer.from("x") });
      expect(upload.ok(), `upload ${key}: ${upload.status()}`).toBeTruthy();
    }

    // Provision the bucket signing key before issuing parallel uploads.
    await createObject(0);
    for (let start = 1; start <= 100; start += 10) {
      await Promise.all(
        Array.from({ length: Math.min(10, 101 - start) }, (_, offset) =>
          createObject(start + offset),
        ),
      );
    }

    await page.getByRole("button", { name: "Buckets" }).click();
    await page.getByText(bucket).first().click();
    await page.getByRole("tab", { name: "Browse" }).click();
    await page.getByLabel("Filter by object key prefix").fill("pagination-");
    await page.getByRole("button", { name: "Find" }).click();
    const rows = page.locator(".explorer-row:not(.explorer-folder)");
    await expect(rows).toHaveCount(100);
    await expect(rows.filter({ hasText: "pagination-000.txt" })).toBeVisible();
    await expect(rows.filter({ hasText: "pagination-100.txt" })).toHaveCount(0);
    await expect(page.getByText("Page 1", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Previous" })).toBeDisabled();
    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.getByText("Page 2", { exact: true })).toBeVisible();
    await expect(rows).toHaveCount(1);
    await expect(rows.filter({ hasText: "pagination-100.txt" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Next" })).toBeDisabled();
    await page.getByRole("button", { name: "Previous" }).click();
    await expect(page.getByText("Page 1", { exact: true })).toBeVisible();
    await expect(rows).toHaveCount(100);
    await page.getByRole("button", { name: "Next" }).click();
    await expect(page.getByText("Page 2", { exact: true })).toBeVisible();
    await page.getByLabel("Filter by object key prefix").fill("pagination-000");
    await page.getByRole("button", { name: "Find" }).click();
    await expect(rows).toHaveCount(1);
    await expect(rows.filter({ hasText: "pagination-000.txt" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Previous" })).toHaveCount(0);
  });
});
