#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
CAPTURE_SCREENSHOTS=true sh "$root/dev/run-e2e.sh"
