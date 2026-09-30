export interface BulkResult<T> {
  item: T
  error?: Error
}

// runBulk applies fn to every item, a few at a time so a large selection
// does not flood the agent, and reports progress after each one.
export async function runBulk<T>(
  items: T[],
  fn: (item: T) => Promise<void>,
  onProgress?: (done: number) => void,
  concurrency = 4,
): Promise<BulkResult<T>[]> {
  const results: BulkResult<T>[] = new Array(items.length)
  let next = 0
  let done = 0
  const worker = async () => {
    while (next < items.length) {
      const i = next++
      try {
        await fn(items[i])
        results[i] = { item: items[i] }
      } catch (e) {
        results[i] = { item: items[i], error: e as Error }
      }
      onProgress?.(++done)
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, items.length) }, worker))
  return results
}
