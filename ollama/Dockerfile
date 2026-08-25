# Bakes the fine-tuned advisory model into the Ollama image at build time, so
# `docker compose up` never needs internet access or a manual `ollama pull`
# (DESIGN.md decision 4: "GGUF di-bake ke image"). The GGUF itself is never
# committed to this repo (261 MB, regenerated from a LoRA checkpoint in
# audiax_model) -- run `make ollama-model` (or scripts/prepare_ollama_model.sh)
# to stage it into ./model/ before building this image.
FROM ollama/ollama:latest

WORKDIR /model
COPY model/audiax-advisor-q4km.gguf ./audiax-advisor-q4km.gguf
COPY Modelfile ./Modelfile

# `ollama create` needs a running `ollama serve` to talk to -- both happen in
# this one RUN step so the daemon's lifetime matches the step. What it writes
# under /root/.ollama persists in the resulting image layer even though the
# server process itself is gone once the step commits.
#
# Fails the build loudly (missing GGUF, daemon never came up, or `ollama
# create` rejects the file) instead of producing an image that looks fine and
# then 404s on the first chat request in front of a judge -- the same class of
# bug the AI repo's Dockerfile was fixed for (its docs/PROGRESS.md, bug 3).
RUN test -s ./audiax-advisor-q4km.gguf \
      || (echo "audiax-advisor-q4km.gguf missing or empty -- run scripts/prepare_ollama_model.sh first" >&2 && exit 1)
#
# The backgrounded `ollama serve` has its stdout/stderr redirected to a file,
# not inherited -- otherwise BuildKit waits for that pipe to close before it
# will consider this RUN step finished, and a daemon that never exits on its
# own means the step (and the whole build) hangs forever after the real work
# is already done.
RUN ollama serve >/tmp/ollama-serve.log 2>&1 & \
    SERVE_PID=$!; \
    for i in $(seq 1 30); do \
      ollama list >/dev/null 2>&1 && break; \
      sleep 1; \
    done; \
    ollama create audiax-advisor -f ./Modelfile; \
    ollama list | grep -q audiax-advisor; \
    STATUS=$?; \
    kill "$SERVE_PID" 2>/dev/null || true; \
    wait "$SERVE_PID" 2>/dev/null || true; \
    exit "$STATUS"

# Reachable by other containers on the compose network at ollama:11434, which
# is also constants.DefaultOllamaURL's host in the Go backend.
ENV OLLAMA_HOST=0.0.0.0:11434
