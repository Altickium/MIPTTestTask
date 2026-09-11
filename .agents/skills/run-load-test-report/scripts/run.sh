#!/bin/sh
set -eu
: "${SCENARIO:=unique}"
: "${VUS:=20}"
: "${DURATION:=20s}"
case "$SCENARIO" in
  unique) DUPLICATE_RATIO=0 ;;
  duplicate) DUPLICATE_RATIO=1 ;;
  burst) : "${DUPLICATE_RATIO:=0.1}" ;;
  *) echo "scenario must be unique, duplicate, or burst" >&2; exit 2 ;;
esac
export VUS DURATION DUPLICATE_RATIO
mkdir -p docs/ai/load-reports
k6 run --summary-export docs/ai/load-reports/latest.json tests/load/vote.js
