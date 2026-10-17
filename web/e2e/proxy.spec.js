import { test, expect } from "@playwright/test";
import { createServer, request as httpRequest } from "node:http";

const proxyURL = process.env.E2E_PROXY_URL;
const upstream = process.env.E2E_PROXY_UPSTREAM;
const untrusted = process.env.E2E_PROXY_UNTRUSTED_UPSTREAM;
const prefix = "/proxy-e2e/";

// This fixture is deliberately restricted to loopback and is not a real authentication proxy.
function testProxy(target) {
  return createServer((client, response) => {
    const headers = { ...client.headers };
    for (const name of [
      "x-forwarded-user",
      "x-auth-request-user",
      "remote-user",
      "x-forwarded-email",
      "x-auth-request-email",
      "x-authentik-email",
      "x-forwarded-name",
      "x-auth-request-preferred-username",
      "x-authentik-name",
      "x-forwarded-groups",
      "x-auth-request-groups",
    ])
      delete headers[name];
    const identity = /(?:^|;\s*)e2e_identity=(admin|viewer)(?:;|$)/.exec(
      headers.cookie || "",
    )?.[1];
    if (identity) {
      headers["x-forwarded-user"] = `proxy-${identity}`;
      if (identity === "admin") headers["x-forwarded-groups"] = "garage-admins";
    }
    const request = httpRequest(
      new URL(client.url, target),
      {
        method: client.method,
        headers: { ...headers, host: new URL(target).host },
      },
      (result) => {
        response.writeHead(result.statusCode, result.headers);
        result.pipe(response);
      },
    );
    request.on("error", () => {
      if (!response.headersSent) response.writeHead(502);
      response.end();
    });
    client.pipe(request);
  });
}

test.describe("trusted proxy authentication", () => {
  test.skip(
    !proxyURL || !upstream,
    "Configure E2E_PROXY_URL and E2E_PROXY_UPSTREAM to run proxy tests",
  );
  let proxy;
  const base = proxyURL ? new URL(prefix, proxyURL).href : "";

  test.beforeAll(async () => {
    const address = new URL(proxyURL);
    if (address.hostname !== "127.0.0.1")
      throw new Error("The test proxy must listen on 127.0.0.1");
    proxy = testProxy(upstream);
    await new Promise((resolve, reject) => {
      proxy.once("error", reject);
      proxy.listen(Number(address.port), "127.0.0.1", resolve);
    });
  });

  test.afterAll(async () => {
    if (proxy) await new Promise((resolve) => proxy.close(resolve));
  });

  test("requires an asserted identity and strips forged client headers", async ({
    page,
  }) => {
    await page.goto(base);
    const session = await page.request.get(new URL("api/session", base).href, {
      headers: {
        "X-Forwarded-User": "forged",
        "X-Forwarded-Groups": "garage-admins",
      },
    });
    expect((await session.json()).user).toBe("");
    expect(
      (await page.request.get(new URL("api/users", base).href)).status(),
    ).toBe(401);
  });

  test("grants the configured admin group and blocks viewer mutations", async ({
    page,
  }) => {
    await page
      .context()
      .addCookies([{ name: "e2e_identity", value: "admin", url: proxyURL }]);
    await page.goto(base);
    await expect(page.getByText("proxy-admin")).toBeVisible();
    await expect(page.getByRole("button", { name: "Users" })).toBeVisible();
    const admin = await page.request.get(new URL("api/session", base).href);
    expect(await admin.json()).toMatchObject({
      user: "proxy-admin",
      role: "admin",
      mode: "proxy",
    });
    expect(
      (await page.request.get(new URL("api/users", base).href)).status(),
    ).toBe(200);

    await page
      .context()
      .addCookies([{ name: "e2e_identity", value: "viewer", url: proxyURL }]);
    await page.reload();
    await expect(page.getByText("proxy-viewer")).toBeVisible();
    await expect(page.getByRole("button", { name: "Users" })).toHaveCount(0);
    const viewer = await page.request.get(new URL("api/session", base).href);
    expect(await viewer.json()).toMatchObject({
      user: "proxy-viewer",
      role: "viewer",
    });
    const forged = await page.request.get(new URL("api/session", base).href, {
      headers: {
        "X-Forwarded-User": "proxy-admin",
        "X-Forwarded-Groups": "garage-admins",
      },
    });
    expect(await forged.json()).toMatchObject({
      user: "proxy-viewer",
      role: "viewer",
    });
    expect(
      (await page.request.get(new URL("api/users", base).href)).status(),
    ).toBe(403);
    const mutation = await page.request.post(
      new URL("api/garage/CreateBucket", base).href,
      {
        headers: {
          Origin: new URL(proxyURL).origin,
          "Content-Type": "application/json",
        },
        data: { globalAlias: "proxy-viewer-forbidden" },
      },
    );
    expect(mutation.status()).toBe(403);
  });

  test("rejects an identity asserted by an untrusted peer", async ({
    request,
  }) => {
    test.skip(
      !untrusted,
      "Configure E2E_PROXY_UNTRUSTED_UPSTREAM to test an untrusted peer",
    );
    const response = await request.get(
      new URL(`${prefix}api/session`, untrusted).href,
      {
        headers: {
          "X-Forwarded-User": "proxy-admin",
          "X-Forwarded-Groups": "garage-admins",
        },
      },
    );
    expect((await response.json()).user).toBe("");
    const users = await request.get(
      new URL(`${prefix}api/users`, untrusted).href,
      {
        headers: {
          "X-Forwarded-User": "proxy-admin",
          "X-Forwarded-Groups": "garage-admins",
        },
      },
    );
    expect(users.status()).toBe(401);
  });
});
