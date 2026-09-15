#!/usr/bin/env bash
set -e

# ====== read git info ======
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
COMMIT=$(git rev-parse --short=8 HEAD 2>/dev/null || echo unknown)

[ -z "$BRANCH" ] && BRANCH=unknown
[ -z "$COMMIT" ] && COMMIT=unknown

echo "Branch: $BRANCH"
echo "Commit: $COMMIT"

# ====== go build with ldflags ======
go build -ldflags "-X main.gitBranch=$BRANCH -X main.gitCommit=$COMMIT" -o gmk .

echo "Build ok: gmk ${BRANCH}@${COMMIT}"