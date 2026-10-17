import { test, expect } from "@playwright/test";

const base = process.env.E2E_OIDC_URL;

async function signIn(page, user) {
  await page.goto(base);
  await page
    .getByRole("link", { name: "Continue with your identity provider" })
    .click();
  await page.locator("#username").fill(user);
  await page.locator("#password").fill("local-test-password");
  await page.locator("#kc-login").click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
}

test.describe("OIDC authentication with Keycloak", () => {
  test.skip(!base, "Configure E2E_OIDC_URL to run Keycloak tests");

  test("signs in with an admin group and revokes the application session", async ({
    page,
  }) => {
    await signIn(page, "oidc-admin");
    await expect(page.getByText("oidc-admin")).toBeVisible();
    await expect(page.getByRole("button", { name: "Users" })).toBeVisible();
    const session = await page.request.get(new URL("api/session", base).href);
    expect(await session.json()).toMatchObject({
      mode: "oidc",
      user: "oidc-admin",
      role: "admin",
    });
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(
      (cookie) => cookie.name === "garage_webui_session",
    );
    expect(sessionCookie).toMatchObject({ httpOnly: true, path: "/oidc-e2e/" });
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(
      page.getByRole("link", { name: "Continue with your identity provider" }),
    ).toBeVisible();
    await page.context().addCookies([sessionCookie]);
    expect(
      (await page.request.get(new URL("api/users", base).href)).status(),
    ).toBe(401);
  });

  test("signs in a viewer without admin permissions", async ({ page }) => {
    await signIn(page, "oidc-viewer");
    await expect(page.getByText("oidc-viewer")).toBeVisible();
    await expect(page.getByRole("button", { name: "Users" })).toHaveCount(0);
    const session = await page.request.get(new URL("api/session", base).href);
    expect(await session.json()).toMatchObject({
      mode: "oidc",
      user: "oidc-viewer",
      role: "viewer",
    });
    expect(
      (await page.request.get(new URL("api/users", base).href)).status(),
    ).toBe(403);
  });

  test("rejects callbacks without a valid state", async ({ request }) => {
    const callback = await request.get(
      new URL("auth/callback?code=invalid&state=invalid", base).href,
    );
    expect(callback.status()).toBe(401);
  });
});
