# Idea: Parallel Batch Testing

## Context
`SubscriptionService.compute_top_servers` iterates through batches sequentially:
```python
for i in range(0, len(servers), self.settings.BATCH_SIZE):
    await self.xray_service.run_test_batch(batch)
```
If we have 1000 servers and batch size is 20 (taking 5-10s), it takes ~4-8 minutes.

## Proposal
Use `asyncio.Semaphore` to run multiple batches concurrently.

## Implementation Steps
1.  Define `MAX_CONCURRENT_BATCHES` (e.g., 3 or 5).
2.  Refactor the loop to create tasks:
    ```python
    sem = asyncio.Semaphore(MAX_CONCURRENT_BATCHES)
    
    async def run_guarded_batch(batch):
        async with sem:
            return await self.xray_service.run_test_batch(batch)
    
    tasks = [run_guarded_batch(batch) for batch in batches]
    results = await asyncio.gather(*tasks)
    ```

## Risks
-   **CPU/Port Exhaustion:** Running too many local Xray instances might spike CPU or run out of ports. Needs careful tuning of `BATCH_SIZE` vs `MAX_CONCURRENT_BATCHES`.

## Benefits
-   **Speed:** Drastically reduce the time to refresh the server list.
