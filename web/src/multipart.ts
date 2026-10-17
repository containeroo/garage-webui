// abortMultipart preserves the original failure and reports unconfirmed cleanup.
export async function abortMultipart(
  cleanup: () => Promise<Response>,
  failure: unknown,
): Promise<never> {
  try {
    const response = await cleanup();
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
  } catch {
    throw new Error(
      `${failure instanceof Error ? failure.message : String(failure)}. The incomplete multipart upload could not be cleaned up; check the bucket's lifecycle policy.`,
      { cause: failure },
    );
  }
  throw failure;
}
