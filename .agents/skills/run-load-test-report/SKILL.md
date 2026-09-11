---
name: run-load-test-report
description: Run a reproducible k6 unique, duplicate, or burst vote scenario for this repository and write a factual Markdown performance report.
---

Read [scenario guidance](references/scenarios.md), select the requested scenario, and run `scripts/run.sh` with explicit VUs, duration, and duplicate ratio. The API stack must already be healthy.

Create a report from [the template](assets/report-template.md). Include date, commit if available, hardware/environment, service settings, exact command, RPS, p50/p95/p99, error rate, and the observed bottleneck. Link the raw k6 summary. Never invent numbers when a run was skipped or failed.
