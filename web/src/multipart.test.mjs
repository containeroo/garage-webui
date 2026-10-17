import assert from "node:assert/strict";
import test from "node:test";
import { abortMultipart } from "./multipart.ts";

test("preserves the original error when abort succeeds", async () => {
  const original = new Error("upload failed");
  await assert.rejects(
    abortMultipart(async () => new Response(null, { status: 204 }), original),
    (error) => error === original,
  );
});

test("reports when abort fails or cannot be reached", async () => {
  for (const cleanup of [
    async () => new Response(null, { status: 403 }),
    async () => {
      throw new Error("offline");
    },
  ]) {
    await assert.rejects(
      abortMultipart(cleanup, new Error("upload failed")),
      /upload failed.*could not be cleaned up/,
    );
  }
});
