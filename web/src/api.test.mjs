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
    "Invalid request\nuser name: Required",
  );
  const garage = new Response(
    JSON.stringify({ message: "Bucket already exists" }),
    { status: 409 },
  );
  assert.equal(
    errorMessage(await responseProblem(garage)),
    "Bucket already exists",
  );
});

test("handles text, empty, malformed and HTML errors", async () => {
  for (const [body, headers, expected] of [
    ["Access denied", {}, "Access denied"],
    ["", {}, "Request failed (HTTP 502)."],
    ['{"other":true}', {}, "Request failed (HTTP 502)."],
    [
      "<html>proxy failure</html>",
      { "Content-Type": "text/html" },
      "Request failed (HTTP 502).",
    ],
  ]) {
    const response = new Response(body, { status: 502, headers });
    assert.equal(errorMessage(await responseProblem(response)), expected);
  }
});
