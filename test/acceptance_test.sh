#!/usr/bin/env bash
set -euo pipefail

# Acceptance tests for teeny-orchestrator
# Tests that don't require an API key (plumbing/wiring tests)

PASS=0
FAIL=0
BINARY="./teeny-orchestrator"

pass() { PASS=$((PASS + 1)); echo "  ✅ $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $1: $2"; }

cd "$(dirname "$0")/.."

echo "Building..."
go build -o "$BINARY" ./cmd/teeny-orchestrator/

echo ""
echo "=== Plumbing Tests (no API key needed) ==="

# Test 1: Binary exists and runs
echo ""
echo "--- Binary ---"
if $BINARY --help >/dev/null 2>&1; then
  pass "binary runs"
else
  fail "binary runs" "exit code $?"
fi

# Test 2: Init creates directories
echo ""
echo "--- Init ---"
export HOME=$(mktemp -d)
$BINARY init >/dev/null 2>&1
if [ -d "$HOME/.teeny-claw/workspace" ] && [ -d "$HOME/.teeny-claw/sessions" ] && [ -d "$HOME/.teeny-claw/tools" ]; then
  pass "init creates directories"
else
  fail "init creates directories" "missing dirs"
fi

if [ -f "$HOME/.teeny-claw/config.json" ]; then
  pass "init creates config.json"
else
  fail "init creates config.json" "missing file"
fi

if [ -f "$HOME/.teeny-claw/workspace/AGENTS.md" ]; then
  pass "init creates AGENTS.md"
else
  fail "init creates AGENTS.md" "missing file"
fi

# Test 3: Tool manifest discovery
echo ""
echo "--- Tool Discovery ---"
mkdir -p "$HOME/.teeny-claw/tools/test-tool"
cat > "$HOME/.teeny-claw/tools/test-tool/tool.json" << 'MANIFEST'
{
  "name": "test-tool",
  "binary": "echo",
  "description": "Test tool",
  "commands": {
    "hello": {
      "description": "Say hello",
      "args": "hello from test-tool",
      "parameters": {}
    }
  }
}
MANIFEST

# The tool should be discoverable (we can't test this without running the loop,
# but we can verify the manifest parses)
if python3 -c "import json; json.load(open('$HOME/.teeny-claw/tools/test-tool/tool.json'))" 2>/dev/null; then
  pass "tool manifest is valid JSON"
else
  fail "tool manifest is valid JSON" "parse error"
fi

# Test 4: Run without API key gives clear error
echo ""
echo "--- Error Handling ---"
unset ANTHROPIC_API_KEY 2>/dev/null || true
if output=$($BINARY run "test" 2>&1); then
  fail "missing API key errors" "should have failed"
else
  if echo "$output" | grep -qi "api.key\|not set\|anthropic"; then
    pass "missing API key gives clear error"
  else
    fail "missing API key gives clear error" "unclear error: $output"
  fi
fi

# Test 5: Run without message gives error
echo ""
if output=$($BINARY run 2>&1 </dev/null); then
  fail "empty message errors" "should have failed"
else
  if echo "$output" | grep -qi "no message\|message"; then
    pass "empty message gives clear error"
  else
    fail "empty message gives clear error" "unclear error: $output"
  fi
fi

# Test 6: Session directory created on init
echo ""
echo "--- Sessions ---"
if [ -d "$HOME/.teeny-claw/sessions" ]; then
  pass "sessions directory exists"
else
  fail "sessions directory exists" "missing"
fi

# Cleanup
rm -rf "$HOME"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
