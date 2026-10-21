import { test, expect } from "@playwright/test";
import { mkdtemp, open, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
const origin = new URL(process.env.E2E_BASE_URL).origin;
const headers = { Origin: origin, "Content-Type": "application/json" };
const bucket = `e2e-${Date.now()}`;
let administratorCookies;
async function login(page, token = process.env.E2E_ADMIN_TOKEN) {
  await page.goto("./");
  await page.getByLabel("Admin token").fill(token);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}
// confirmAdminTokenPrivileges accepts the explicit escalation warning for admin tokens.
async function confirmAdminTokenPrivileges(page) {
  const dialog = page.getByRole("dialog", { name: "Please confirm" });
  await expect(dialog).toContainText(
    "This token can grant full Garage administration permissions.",
  );
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(dialog).toBeHidden();
}
async function garage(operation, body, query = "") {
  const response = await fetch(
    `${process.env.E2E_GARAGE_URL}/v2/${operation}${query}`,
    {
      method: body ? "POST" : "GET",
      headers: {
        Authorization: `Bearer ${process.env.E2E_ADMIN_TOKEN}`,
        "Content-Type": "application/json",
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    },
  );
  expect(response.ok, await response.clone().text()).toBeTruthy();
  const text = await response.text();
  return text ? JSON.parse(text) : undefined;
}
test.describe.serial("Garage credentials and storage", () => {
  test("signs in with an admin token and creates a bucket", async ({
    page,
  }) => {
    await login(page);
    administratorCookies = await page.context().cookies();
    await page.getByRole("button", { name: "Buckets", exact: true }).click();
    await page.getByRole("button", { name: "Create bucket" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Name").fill(bucket);
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(bucket).first()).toBeVisible();
  });
  test("creates, edits and revokes a named admin token through custom dialogs", async ({
    page,
  }) => {
    await login(page);
    await page
      .getByRole("button", { name: "Admin tokens", exact: true })
      .click();
    await page.getByRole("button", { name: "Create token" }).click();
    let dialog = page.getByRole("dialog", { name: "Create admin token" });
    await dialog.getByLabel("Name", { exact: true }).fill("operator-test");
    await dialog.getByLabel("All operations").check();
    await dialog
      .getByRole("button", { name: "Create token", exact: true })
      .click();
    await confirmAdminTokenPrivileges(page);
    dialog = page.getByRole("dialog", { name: "Save your admin token" });
    const token = await dialog.getByLabel("Secret token").inputValue();
    expect(token.length).toBeGreaterThan(20);
    await dialog.getByRole("button", { name: "Done", exact: true }).click();
    const row = page.locator(".table-row").filter({ hasText: "operator-test" });
    await row.getByTitle("Edit token").click();
    dialog = page.getByRole("dialog", { name: "Edit admin token" });
    await dialog.getByLabel("Name", { exact: true }).fill("operator-renamed");
    await dialog.getByRole("button", { name: "Save token" }).click();
    await confirmAdminTokenPrivileges(page);
    await expect(dialog).toBeHidden();
    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, token);
    await expect(page.getByText("operator-renamed").first()).toBeVisible();
    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page);
    await page
      .getByRole("button", { name: "Admin tokens", exact: true })
      .click();
    await page
      .locator(".table-row")
      .filter({ hasText: "operator-renamed" })
      .getByTitle("Revoke token")
      .click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Confirm" })
      .click();
    await expect(
      page.locator(".table-row").filter({ hasText: "operator-renamed" }),
    ).toHaveCount(0);
    administratorCookies = await page.context().cookies();
  });
  test("scope-limited admin tokens cannot manage other tokens", async ({
    page,
  }) => {
    const token = await garage("CreateAdminToken", {
      name: "bucket-reader",
      scope: ["ListBuckets", "GetBucketInfo"],
    });
    await login(page, token.secretToken);
    await expect(
      page.getByRole("button", { name: "Admin tokens", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Cluster", exact: true }),
    ).toHaveCount(0);
    expect(
      (await page.request.get("api/garage/ListAdminTokens")).status(),
    ).toBe(403);
    expect(
      (
        await page.request.post("api/garage/CreateBucket", {
          headers,
          data: { globalAlias: "forbidden" },
        })
      ).status(),
    ).toBe(403);
    await garage("DeleteAdminToken", {}, `?id=${encodeURIComponent(token.id)}`);
    expect((await page.request.get("api/garage/ListBuckets")).status()).toBe(
      401,
    );
  });
  test("S3 keys see only permitted buckets and Garage enforces read-only access", async ({
    page,
  }) => {
    const buckets = await garage("ListBuckets");
    const selected = buckets.find((value) =>
      value.globalAliases.includes(bucket),
    );
    const key = await garage("CreateKey", { name: "s3-reader" });
    await garage("AllowBucketKey", {
      bucketId: selected.id,
      accessKeyId: key.accessKeyId,
      permissions: { read: true },
    });
    // Default browsing must work without bucket CORS.
    await garage(
      "UpdateBucket",
      { corsRules: [] },
      `?id=${encodeURIComponent(selected.id)}`,
    );
    await page.goto("./");
    await page.getByRole("tab", { name: "S3 credentials" }).click();
    await page.getByLabel("Access key ID").fill(key.accessKeyId);
    await page.getByLabel("Secret access key").fill(key.secretAccessKey);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Admin tokens", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Cluster", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Access keys", exact: true }),
    ).toHaveCount(0);
    const listing = await page.request.get("api/garage/ListBuckets");
    expect(listing.ok()).toBeTruthy();
    expect((await listing.json()).map((value) => value.id)).toEqual([bucket]);
    expect(
      (
        await page.request.get("api/garage/GetBucketInfo?id=unrelated")
      ).status(),
    ).toBe(403);
    expect((await page.request.get("api/garage/ListKeys")).status()).toBe(403);
    const signed = await page.request.post("api/uploads/presign", {
      headers,
      data: { bucketId: bucket, key: "forbidden.txt", size: 1 },
    });
    expect(signed.ok()).toBeTruthy();
    const { url } = await signed.json();
    expect((await page.request.put(url, { data: "x" })).status()).toBe(403);
    const read = await page.request.post("api/objects/presign", {
      headers,
      data: { bucketId: bucket, action: "list" },
    });
    expect(read.ok()).toBeTruthy();
    expect((await page.request.get((await read.json()).url)).ok()).toBeTruthy();
    await page.getByText(bucket, { exact: true }).first().click();
    await expect(
      page.getByRole("tab", { name: "Overview", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("tab", { name: "Permissions", exact: true }),
    ).toHaveCount(0);
    await expect(page.getByText("Nothing here")).toBeVisible();
    const cookies = await page.context().cookies();
    await page.getByRole("button", { name: "Sign out" }).click();
    await page.context().addCookies(cookies);
    expect((await page.request.get("api/garage/ListBuckets")).status()).toBe(
      401,
    );
  });
  test("recovers from a failed session request", async ({ page }) => {
    await page.route("**/api/session", (route) => route.abort(), { times: 1 });
    await page.goto("./");
    await expect(page.getByRole("alert")).toContainText("could not connect");
    await page.getByRole("button", { name: "Try again" }).click();
    await expect(
      page.getByRole("heading", { name: "Sign in to Garage" }),
    ).toBeVisible();
  });

  test("allows retrying a failed upload", async ({ page }) => {
    await page.context().addCookies(administratorCookies);
    await page.goto("./");
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
      "**/api/objects/transfer?*",
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
    await page.context().addCookies(administratorCookies);
    await page.goto("./");
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

  for (const direct of [false, true]) {
    test(`completes a ${direct ? "direct" : "backend"} multipart upload`, async ({
      page,
    }) => {
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
        await page.context().addCookies(administratorCookies);
        await page.goto("./");
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
            (direct
              ? response.url().includes("uploadId=")
              : response.url().includes("/api/objects/transfer?") &&
                JSON.parse(new URL(response.url()).searchParams.get("request"))
                  .action === "multipart-complete"),
        );
        await dialog
          .getByRole("button", {
            name: direct ? "Upload directly to S3" : "Upload file",
            exact: true,
          })
          .click();
        expect((await complete).ok()).toBeTruthy();
        await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
        await dialog
          .getByRole("button", { name: "Close", exact: true })
          .click();
        await expect(
          page
            .locator(".explorer-row:not(.explorer-folder)")
            .filter({ hasText: "multipart.bin" }),
        ).toBeVisible();
      } finally {
        await rm(dir, { recursive: true, force: true });
      }
    });
  }
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
      await page.context().addCookies(administratorCookies);
      await page.goto("./");
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
          route.request().url().includes("/api/objects/transfer?") &&
          JSON.parse(new URL(route.request().url()).searchParams.get("request"))
            .action === "multipart-part"
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
          response.url().includes("/api/objects/transfer?") &&
          JSON.parse(new URL(response.url()).searchParams.get("request"))
            .action === "multipart-abort",
      );
      await dialog.getByRole("button", { name: "Cancel uploads" }).click();
      release();
      expect((await abort).ok()).toBeTruthy();
      await expect(dialog.getByText("uploads cancelled")).toBeVisible();
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

test("keeps sign-in anchored and restores the preferred theme after signing out", async ({
  page,
}) => {
  await page.goto("./");
  const card = page.locator(".login-card");
  const adminPosition = await card.boundingBox();
  await page.getByRole("tab", { name: "S3 credentials" }).click();
  await expect(page.getByLabel("Access key ID")).toBeVisible();
  expect((await card.boundingBox()).y).toBe(adminPosition.y);
  await page.getByRole("tab", { name: "Admin token" }).click();
  expect((await card.boundingBox()).y).toBe(adminPosition.y);
  await login(page);
  await page.getByRole("button", { name: /^My profile:/ }).click();
  await page.getByLabel("Theme").selectOption("Dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Sign in to Garage" }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Sign in to Garage" }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await login(page);
  await page.getByRole("button", { name: /^My profile:/ }).click();
  await expect(page.getByLabel("Theme")).toHaveValue("Dark");
});

test("streams S3-key uploads and downloads without bucket CORS", async ({
  page,
}) => {
  const alias = `backend-transfer-${Date.now()}`;
  const created = await garage("CreateBucket", { globalAlias: alias });
  const key = await garage("CreateKey", { name: "backend-transfer-user" });
  await garage("AllowBucketKey", {
    bucketId: created.id,
    accessKeyId: key.accessKeyId,
    permissions: { read: true, write: true },
  });
  await page.goto("./");
  await page.getByRole("tab", { name: "S3 credentials" }).click();
  await page.getByLabel("Access key ID").fill(key.accessKeyId);
  await page.getByLabel("Secret access key").fill(key.secretAccessKey);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByText(alias, { exact: true }).first().click();
  await expect(page.getByText("Nothing here")).toBeVisible();
  await page
    .getByRole("button", { name: /upload/i })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.locator('input[type="file"]').setInputFiles({
    name: "backend.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("streamed without CORS"),
  });
  await dialog
    .getByRole("button", { name: "Upload file", exact: true })
    .click();
  await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  const row = page.locator(".explorer-row").filter({ hasText: "backend.txt" });
  await expect(row).toBeVisible();
  await row.getByTitle("Preview").click();
  await expect(
    page.getByText("streamed without CORS", { exact: true }),
  ).toBeVisible();
  const download = page.waitForEvent("download");
  await row.getByTitle("Download", { exact: true }).click();
  expect((await download).url()).toContain("/api/objects/transfer?");
  const info = await garage("GetBucketInfo", undefined, `?id=${created.id}`);
  expect(info.corsRules || []).toEqual([]);
});

test("offers explicit signed S3 uploads and downloads", async ({ page }) => {
  const alias = `direct-transfer-${Date.now()}`;
  const created = await garage("CreateBucket", { globalAlias: alias });
  await login(page);
  await page.getByRole("button", { name: "Buckets", exact: true }).click();
  await page.getByText(alias, { exact: true }).first().click();
  await page.getByRole("tab", { name: "Browse" }).click();
  await page
    .getByRole("button", { name: /upload/i })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.locator('input[type="file"]').setInputFiles({
    name: "direct.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("direct upload"),
  });
  const put = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      response.url().includes("X-Amz-Signature="),
  );
  await dialog
    .getByRole("button", { name: "Upload directly to S3", exact: true })
    .click();
  expect((await put).ok()).toBeTruthy();
  await expect(dialog.getByText("1 / 1 completed")).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByRole("button", { name: "direct.txt", exact: true }).click();
  const download = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download directly from S3", exact: true })
    .click();
  expect((await download).url()).toContain("X-Amz-Signature=");
  const info = await garage("GetBucketInfo", undefined, `?id=${created.id}`);
  expect(
    info.corsRules.some((rule) => rule.AllowedOrigin.includes(origin)),
  ).toBeTruthy();
});
