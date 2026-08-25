#!/usr/bin/env bash
# Copies the fine-tuned advisory GGUF from the sibling audiax_model repo into
# ollama/model/, where ollama/Dockerfile expects to find it before `docker
# compose build` runs. The file itself is never committed here (261 MB,
# regenerated from a LoRA checkpoint) -- see audiax_model's
# experiments/advisory/dist/README.md for the from-scratch build recipe.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODEL_NAME="audiax-advisor-q4km.gguf"

SRC="${AUDIAX_MODEL_REPO:-$REPO_ROOT/../audiax_model}/experiments/advisory/dist/$MODEL_NAME"
DEST_DIR="$REPO_ROOT/ollama/model"
DEST="$DEST_DIR/$MODEL_NAME"

# A 261 MB file that got cut off mid-copy is still "present" -- checking only
# existence would let a corrupt artifact through silently, the same failure
# mode the AI repo's download_assets.py was fixed for (its PROGRESS.md, bug
# 4) before it started enforcing a minimum size and exiting non-zero.
MIN_BYTES=200000000
EXPECTED_SHA256="2108fbd9f9f2219a6fdbb5bc4a6be4bf667d0a6a7eb7aa52ab71eefa8e7d8d16"

if [ ! -f "$SRC" ]; then
  echo "error: $MODEL_NAME not found at $SRC" >&2
  echo "" >&2
  echo "Regenerate it from audiax_model/experiments/advisory/:" >&2
  echo "  python merge_ckpt.py --step 60 --out merged_e60" >&2
  echo "  python <llama.cpp>/convert_hf_to_gguf.py merged_e60 --outfile adv-f16.gguf --outtype f16" >&2
  echo "  llama-quantize adv-f16.gguf $MODEL_NAME Q4_K_M" >&2
  echo "" >&2
  echo "Or set AUDIAX_MODEL_REPO to point at a clone of audiax_model." >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
cp "$SRC" "$DEST"

ACTUAL_BYTES=$(wc -c < "$DEST")
if [ "$ACTUAL_BYTES" -lt "$MIN_BYTES" ]; then
  echo "error: $DEST is only $ACTUAL_BYTES bytes (expected >= $MIN_BYTES) -- copy is truncated or wrong file" >&2
  rm -f "$DEST"
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_SHA256=$(sha256sum "$DEST" | cut -d' ' -f1)
  if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
    echo "warning: sha256 mismatch for $DEST" >&2
    echo "  expected $EXPECTED_SHA256 (checkpoint-60, per audiax_model/experiments/advisory/dist/README.md)" >&2
    echo "  got      $ACTUAL_SHA256" >&2
    echo "If you retrained or requantized, this is expected -- the size check above is the hard gate." >&2
  fi
fi

echo "ok: $DEST ($ACTUAL_BYTES bytes)"
