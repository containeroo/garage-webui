import { chromium, expect } from "@playwright/test";
import { join } from "node:path";

const browser = await chromium.launch({
  headless: true,
  ...(process.env.E2E_CHROMIUM_PATH && {
    executablePath: process.env.E2E_CHROMIUM_PATH,
  }),
});
try {
  const page = await browser.newPage({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
  });
  await page.goto(process.env.SCREENSHOT_URL);
  await page.getByLabel("Username").fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("documentation-password");
  await page.getByLabel("Confirm password").fill("documentation-password");
  await page.getByRole("button", { name: "Create administrator" }).click();
  await page.getByRole("button", { name: "Buckets", exact: true }).click();

  for (const name of ["project-assets", "team-backups", "website-media"]) {
    await page.getByRole("button", { name: "Create bucket" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Name").fill(name);
    await dialog.getByRole("button", { name: "Create", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(name).first()).toBeVisible();
  }
  const capture = (name) =>
    page.screenshot({
      path: join(process.env.SCREENSHOT_DIR, `${name}.png`),
      animations: "disabled",
    });
  await capture("buckets");

  await page.getByRole("button", { name: "Overview", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await expect(page.getByText("website-media").first()).toBeVisible();
  await capture("overview");

  await page.getByRole("button", { name: "Cluster", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Cluster", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Connected", { exact: true }).first(),
  ).toBeVisible();
  await capture("cluster");
} finally {
  await browser.close();
}
