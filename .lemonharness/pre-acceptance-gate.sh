#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────
# LemonHarness Pre-Acceptance Quality Gate
# Run BEFORE accepting sub-agent output or declaring a task complete.
# Focused checks: file size limits, dead code markers, TODO/FIXME/HACK,
# basic syntax validation, and complexity red flags.
#
# Usage: bash .lemonharness/pre-acceptance-gate.sh [targets...]
#   Default: scans src/ .pi/ .lemonharness/ (excludes node_modules .git)
#
# Part of general-rules Rule 8 — mandatory pre-acceptance gate.
# ─────────────────────────────────────────────────────────────────────────
set -euo pipefail

# Default scan targets
SCAN_TARGETS=("${@:-src .pi .lemonharness}")
PASSED=0
FAILED=0
WARNINGS=0

echo "═══════════════════════════════════════════════════════════════"
echo "  🍋 LemonHarness Pre-Acceptance Quality Gate"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# ── Helper: check if a target directory exists ────────────────────
has_files() {
  local dir="$1"
  [ -d "$dir" ] && find "$dir" -type f \( -name "*.ts" -o -name "*.py" -o -name "*.js" -o -name "*.sh" -o -name "*.json" \) 2>/dev/null | head -1 | grep -q .
}

# ── 1. File Size Check ────────────────────────────────────────────
echo "─── 📏 File Size Check (max 400 lines) ───"
LARGE_FILES=0
for target in "${SCAN_TARGETS[@]}"; do
  if [ ! -d "$target" ]; then continue; fi
  while IFS= read -r -d '' f; do
    lines=$(wc -l < "$f")
    if [ "$lines" -gt 400 ]; then
      echo "  ❌ $f ($lines lines, max 400)"
      LARGE_FILES=$((LARGE_FILES + 1))
    fi
  done < <(set -f; find "$target" -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.py" -o -name "*.cs" -o -name "*.go" -o -name "*.rs" \) -print0 2>/dev/null)
done
if [ "$LARGE_FILES" -gt 0 ]; then
  echo "  ❌ $LARGE_FILES file(s) exceed 400-line limit"
  FAILED=$((FAILED + LARGE_FILES))
else
  echo "  ✅ All files within size limits"
fi
echo ""

# ── 2. Dead Code & TODO/FIXME/HACK Check ──────────────────────────
echo "─── 🔍 Dead Code / TODO / FIXME / HACK Check ───"
DIRTY_COUNT=0
TODO_COUNT=0
for target in "${SCAN_TARGETS[@]}"; do
  [ ! -d "$target" ] && continue
  # Check for TODO/FIXME/HACK in source files (exclude memory and node_modules)
  while IFS= read -r -d '' f; do
    matches=$(grep -n "TODO\|FIXME\|HACK\|XXX" "$f" 2>/dev/null || true)
    if [ -n "$matches" ]; then
      echo "  ⚠  $f:"
      echo "$matches" | head -5 | while IFS= read -r line; do echo "     $line"; done
      TODO_COUNT=$((TODO_COUNT + 1))
    fi
  done < <(set -f; find "$target" -type f \( -name "*.ts" -o -name "*.py" -o -name "*.sh" -o -name "*.js" \) -not -path "*/memory/*" -print0 2>/dev/null)

  # Check for commented-out code blocks (5+ consecutive comment lines)
  while IFS= read -r -d '' f; do
    ext="${f##*.}"
    case "$ext" in
      ts|tsx|js) comment_chars="//";;
      py)        comment_chars="#";;
      sh)        comment_chars="#";;
      *)         continue;;
    esac
    # Look for 5+ consecutive comment lines
    consecutive=$(grep -c "^\s*$comment_chars" "$f" 2>/dev/null || true)
    total_lines=$(wc -l < "$f" 2>/dev/null || echo 0)
    if [ "$total_lines" -gt 0 ] && [ "$consecutive" -gt $((total_lines / 3)) ]; then
      # More than 1/3 of file is comments — could be dead code
      echo "  📝 $f: $consecutive comment lines / $total_lines total (${comment_chars} comments)"
      DIRTY_COUNT=$((DIRTY_COUNT + 1))
    fi
  done < <(set -f; find "$target" -type f \( -name "*.ts" -o -name "*.py" -o -name "*.sh" \) -not -path "*/memory/*" -print0 2>/dev/null)
done

# Check for any TODO/FIXME/HACK at project level
PROJECT_TODOS=$(grep -rn "FIXME\|TODO\|HACK\|XXX" . --include="*.ts" --include="*.py" --include="*.sh" --include="*.js" --include="*.json" 2>/dev/null | grep -v "node_modules" | grep -v ".git/" | grep -v ".lemonharness/memory/" | grep -v "package-lock.json" | wc -l | tr -d ' ')
if [ "$PROJECT_TODOS" -gt 0 ]; then
  echo "  ⚠  $PROJECT_TODOS TODO/FIXME/HACK markers found across project"
  WARNINGS=$((WARNINGS + 1))
fi

if [ "$TODO_COUNT" -gt 0 ]; then
  FAILED=$((FAILED + TODO_COUNT))
fi
if [ "$TODO_COUNT" -eq 0 ] && [ "$DIRTY_COUNT" -eq 0 ]; then
  echo "  ✅ No dead code or TODO/FIXME/HACK markers"
fi
echo ""

# ── 3. Syntax / Parse Check ───────────────────────────────────────
echo "─── ⚡ Syntax Check ───"
SYNTAX_FAILS=0

for target in "${SCAN_TARGETS[@]}"; do
  [ ! -d "$target" ] && continue

  # TypeScript: check with tsc --noEmit if available
  if command -v npx &>/dev/null && [ -f "node_modules/.bin/tsc" ]; then
    # Check if directory has .ts files
    if find "$target" -name "*.ts" 2>/dev/null | head -1 | grep -q .; then
      echo "  TypeScript check (tsc --noEmit)..."
      if ! TSC_OUT=$(npx tsc --noEmit 2>&1); then
        TSC_ERR=$(echo "$TSC_OUT" | grep -c "error TS" || true)
        if [ "$TSC_ERR" -gt 0 ]; then
          echo "    Found $TSC_ERR type errors in $target"
          SYNTAX_FAILS=$((SYNTAX_FAILS + TSC_ERR))
        fi
      fi
    fi
  fi

  # Shell scripts: check with bash -n
  while IFS= read -r -d '' f; do
    if ! bash -n "$f" 2>/dev/null; then
      echo "  ❌ Syntax error in $f"
      SYNTAX_FAILS=$((SYNTAX_FAILS + 1))
    fi
  done < <(set -f; find "$target" -name "*.sh" -print0 2>/dev/null)

  # Python: check with python -m py_compile
  while IFS= read -r -d '' f; do
    if ! python3 -m py_compile "$f" 2>/dev/null; then
      echo "  ❌ Syntax error in $f"
      SYNTAX_FAILS=$((SYNTAX_FAILS + 1))
    fi
  done < <(set -f; find "$target" -name "*.py" -print0 2>/dev/null) || true
done

if [ "$SYNTAX_FAILS" -gt 0 ]; then
  echo "  ❌ $SYNTAX_FAILS syntax error(s)"
  FAILED=$((FAILED + SYNTAX_FAILS))
else
  echo "  ✅ All files pass syntax check"
fi
echo ""

# ── 4. Complexity Red Flags ───────────────────────────────────────
echo "─── 🔄 Complexity Red Flags ───"
COMPLEX_FAILS=0

# Check for functions with excessive nesting/indentation
for target in "${SCAN_TARGETS[@]}"; do
  [ ! -d "$target" ] && continue
  while IFS= read -r -d '' f; do
    # Count functions with > 3 levels of indentation (rough complexity proxy)
    deep_lines=$(grep -c "^        " "$f" 2>/dev/null || true)  # 8+ spaces indentation
    total=$(wc -l < "$f" 2>/dev/null || echo 0)
    if [ "$total" -gt 0 ] && [ "$deep_lines" -gt $((total / 4)) ]; then
      echo "  📝 $f: heavy nesting detected ($deep_lines deeply indented lines)"
      COMPLEX_FAILS=$((COMPLEX_FAILS + 1))
    fi
  done < <(set -f; find "$target" -type f \( -name "*.ts" -o -name "*.py" \) -print0 2>/dev/null)
done

if [ "$COMPLEX_FAILS" -gt 0 ]; then
  WARNINGS=$((WARNINGS + COMPLEX_FAILS))
  echo "  ⚠  $COMPLEX_FAILS file(s) with heavy nesting — review manually"
else
  echo "  ✅ No immediate complexity red flags"
fi
echo ""

# ── Summary ───────────────────────────────────────────────────────
echo "═══════════════════════════════════════════════════════════════"
echo "  Results: ● $PASSED passed  ✗ $FAILED failed  ⚠ $WARNINGS warnings"
echo "═══════════════════════════════════════════════════════════════"

# Give actionable advice
if [ "$FAILED" -gt 0 ]; then
  echo ""
  echo "  ❌ Pre-Acceptance Gate FAILED."
  echo "     Fix the issues above before accepting this work."
  echo "     Common fixes:"
  echo "       • Split large files (>400 lines) into modules"
  echo "       • Replace TODO/FIXME with tracked issues"
  echo "       • Remove commented-out dead code"
  echo "       • Fix syntax errors before committing"
  exit 1
elif [ "$WARNINGS" -gt 0 ]; then
  echo ""
  echo "  ⚠  Pre-Acceptance Gate PASSED with warnings."
  echo "     Address warnings when practical, but no blockers."
  exit 0
else
  echo ""
  echo "  ✅ Pre-Acceptance Gate PASSED — ready to accept."
  exit 0
fi
