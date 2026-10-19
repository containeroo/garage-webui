import assert from "node:assert/strict";
import test from "node:test";
import { errorMessage, responseProblem } from "./api.ts";

test("formats structured problems and Garage errors", async () => {
  const problem = new Response(
    JSON.stringify({
      error: "Invalid request",
      problems: { user_name: "Required" },
    }),
    { status: 400, headers: { "Content-Type": "application/json" } },
  );
  assert.equal(
    errorMessage(await responseProblem(problem)),
    "invalid request\nuser name: Required",
  );
  const garage = new Response(
    JSON.stringify({ message: "Bucket already exists" }),
    { status: 409 },
  );
  assert.equal(
    errorMessage(await responseProblem(garage)),
    "bucket already exists",
  );
});

test("handles text, empty, malformed and HTML errors", async () => {
  for (const [body, headers, expected] of [
    ["Access denied", {}, "access denied"],
    ["", {}, "request failed (HTTP 502)."],
    ['{"other":true}', {}, "request failed (HTTP 502)."],
    [
      "<html>proxy failure</html>",
      { "Content-Type": "text/html" },
      "request failed (HTTP 502).",
    ],
  ]) {
    const response = new Response(body, { status: 502, headers });
    assert.equal(errorMessage(await responseProblem(response)), expected);
  }
});

test("normalizes upstream error prefixes without changing the detail", () => {
  assert.equal(errorMessage(new Error("Garage refused access")), "garage refused access");
  assert.equal(errorMessage("  HTTPS failed"), "  https failed");
  assert.equal(errorMessage(new Error("Échec de la connexion")), "échec de la connexion");
});
