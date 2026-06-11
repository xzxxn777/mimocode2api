#!/bin/bash
set -e

echo "[mimo2api] Starting MiMoCode2API proxy..."

# Install MiMo CLI if not present
if ! command -v mimo &> /dev/null; then
    echo "[mimo2api] Installing @mimo-ai/cli..."
    npm install -g @mimo-ai/cli || {
        echo "[mimo2api] WARNING: Failed to install @mimo-ai/cli via npm"
        echo "[mimo2api] Trying curl install..."
        curl -fsSL https://mimo.xiaomi.com/install | bash || true
    }
fi

# Verify mimo is available
if command -v mimo &> /dev/null; then
    echo "[mimo2api] MiMo CLI found at $(which mimo)"
    mimo --version 2>&1 || true
else
    echo "[mimo2api] WARNING: MiMo CLI not found. Backend pool will fail to start."
fi

exec ./mimo2api