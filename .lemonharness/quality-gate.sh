#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────
# LemonHarness Quality Gate — Language-Agnostic
# Run during the Validate phase (P3) to enforce code quality thresholds.
# Auto-detects project language(s) and runs appropriate checks.
#
# Usage: bash .lemonharness/quality-gate.sh [target-directory]
# ─────────────────────────────────────────────────────────────────────────
set -euo pipefail

TARGET="${1:-src}"
PASSED=0
FAILED=0
WARNINGS=0
LANGUAGE=""  # detected below

echo "═══════════════════════════════════════════════════════════════"
echo "  🍋 LemonHarness Quality Gate"
echo "  Scanning: $TARGET"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# ── Language Detection ────────────────────────────────────────────
detect_language() {
  if [ -f "requirements.txt" ] || [ -f "setup.py" ] || [ -f "pyproject.toml" ] || [ -f "Pipfile" ]; then
    echo "  📐 Python project detected"
    LANGUAGE="python"
  elif [ -f "package.json" ]; then
    echo "  📐 Node/TypeScript project detected"
    LANGUAGE="typescript"
  else
    # Fallback: guess by file extensions in target
    if find "$TARGET" -name "*.py" 2>/dev/null | head -1 | grep -q .; then
      echo "  📐 Detected Python files"
      LANGUAGE="python"
    elif find "$TARGET" -name "*.ts" -o -name "*.tsx" 2>/dev/null | head -1 | grep -q .; then
      echo "  📐 Detected TypeScript files"
      LANGUAGE="typescript"
    elif find "$TARGET" -name "*.cs" 2>/dev/null | head -1 | grep -q .; then
      echo "  📐 Detected C# files"
      LANGUAGE="dotnet"
    else
      echo "  ⚠  Could not detect language — running generic checks only"
      LANGUAGE="unknown"
    fi
  fi
  echo ""
}
detect_language

# ── 1. File Size Check (Language-Agnostic) ─────────────────────────
echo "─── 📏 File Size Check ───"
LARGE_FILES=0
if [ -d "$TARGET" ]; then
  EXTENSIONS=""
  case "$LANGUAGE" in
    python)     EXTENSIONS="-name *.py" ;;
    typescript) EXTENSIONS="-name *.ts -o -name *.tsx -o -name *.js" ;;
    # .NET checks removed (not used in this project)
    go)         EXTENSIONS="-name *.go" ;;
    rust)       EXTENSIONS="-name *.rs" ;;
    *)          EXTENSIONS="-name *.py -o -name *.ts -o -name *.tsx -o -name *.js -o -name *.cs -o -name *.go -o -name *.rs" ;;
  esac

  while IFS= read -r -d '' f; do
    lines=$(wc -l < "$f")
    if [ "$lines" -gt 400 ]; then
      echo "  ⚠  $f ($lines lines, max 400)"
      LARGE_FILES=$((LARGE_FILES + 1))
    elif [ "$lines" -gt 200 ]; then
      echo "  📝 $f ($lines lines)"
    fi
  done < <(set -f; find "$TARGET" -type f \( $EXTENSIONS \) -print0 2>/dev/null)

  # Also scan .pi/extensions/ for TypeScript projects (main code location)
  if [ "$LANGUAGE" = "typescript" ] && [ -d ".pi/extensions" ]; then
    while IFS= read -r -d '' f; do
      lines=$(wc -l < "$f")
      if [ "$lines" -gt 400 ]; then
        echo "  ⚠  $f ($lines lines, max 400)"
        LARGE_FILES=$((LARGE_FILES + 1))
      elif [ "$lines" -gt 200 ]; then
        echo "  📝 $f ($lines lines)"
      fi
    done < <(set -f; find .pi/extensions -type f \( $EXTENSIONS \) -print0 2>/dev/null)
  fi
fi
if [ "$LARGE_FILES" -gt 0 ]; then
  echo "  ❌ $LARGE_FILES file(s) exceed 400 lines — consider splitting"
  FAILED=$((FAILED + LARGE_FILES))
else
  echo "  ✅ All files within size limits"
fi
echo ""

# ── 2. Extension Factory Check ────────────────────────────────────
echo "─── 🏭 Extension Factory Check ───"
EXT_BAD=0
# Check that all extension directories have index.ts with export default function
if [ -d ".pi/extensions" ]; then
  # Check each subdirectory entry point
  while IFS= read -r -d '' dir; do
    index_file="$dir/index.ts"
    dirname_f=$(basename "$dir")
    if [ -f "$index_file" ]; then
      if ! grep -q "export default function" "$index_file" 2>/dev/null; then
        echo "  ❌ $dirname_f/index.ts — missing 'export default function'"
        EXT_BAD=$((EXT_BAD + 1))
      fi
    else
      echo "  ❌ $dirname_f/ — no index.ts entry point"
      EXT_BAD=$((EXT_BAD + 1))
    fi
  done < <(find .pi/extensions -mindepth 1 -maxdepth 1 -type d -print0 2>/dev/null)
  # Check no .ts files exist directly in .pi/extensions/ root
  while IFS= read -r -d '' f; do
    echo "  ❌ $(basename "$f") — .ts file in .pi/extensions/ root; move into a subdirectory"
    EXT_BAD=$((EXT_BAD + 1))
  done < <(find .pi/extensions -maxdepth 1 -type f -name "*.ts" -print0 2>/dev/null)
  if [ "$EXT_BAD" -gt 0 ]; then
    echo "  ❌ $EXT_BAD extension structure issue(s)"
    FAILED=$((FAILED + EXT_BAD))
  else
    echo "  ✅ All extension directories have valid index.ts entry points"
  fi
fi

# ── 2b. Syntax Parse Check ──────────────────────────────────────
# Standalone parse check that catches syntax errors before tsc.
# For TypeScript: uses tsc --noEmit (proper TS parser).
# For JavaScript: uses node --check (fast, node-native).
# This is independent of type errors, so parse failures don't get
# lost among pre-existing type issues.
echo "─── ⚡ Syntax Parse Check ───"
PARSE_BAD=0
if [ "$LANGUAGE" = "typescript" ] && command -v npx &>/dev/null; then
  # TypeScript: use tsc --noEmit for proper syntax + type stripping
  TSC_OUTPUT=$(npx tsc --noEmit 2>&1 || true)
  if [ -z "$TSC_OUTPUT" ]; then
    echo "  ✅ All .ts files pass syntax parse check (tsc --noEmit)"
  else
    # Filter for actual parse/syntax errors vs type errors
    SYNTAX_LINES=$(echo "$TSC_OUTPUT" | grep -iE "error TS[0-9]+.*parse|error TS[0-9]+.*syntax|Cannot find name|Unexpected token" | head -10 || true)
    SYNTAX_COUNT=$(echo "$SYNTAX_LINES" | grep -c . || true)
    if [ "$SYNTAX_COUNT" -gt 0 ]; then
      echo "$SYNTAX_LINES"
      echo "  ❌ $SYNTAX_COUNT syntax/parse error(s) from tsc"
      PARSE_BAD=$SYNTAX_COUNT
      FAILED=$((FAILED + PARSE_BAD * 10))
    else
      # Non-syntax errors (type errors) are handled by Type Check section
      echo "  ✅ All .ts files pass syntax parse check"
    fi
  fi
elif command -v node &>/dev/null; then
  # JavaScript: use node --check for .js files only
  while IFS= read -r -d '' f; do
    basename_f=$(basename "$f")
    if ! node --check "$f" 2>/dev/null; then
      PARSE_ERR=$(node --check "$f" 2>&1 | head -3)
      echo "  ❌ $basename_f — parse error"
      echo "     $PARSE_ERR"
      PARSE_BAD=$((PARSE_BAD + 1))
    fi
  done < <(find .pi/extensions -maxdepth 3 -type f -name "*.js" -not -path "*/node_modules/*" -print0 2>/dev/null)
  if [ "$PARSE_BAD" -gt 0 ]; then
    echo "  ❌ $PARSE_BAD file(s) with parse errors"
    FAILED=$((FAILED + PARSE_BAD * 10))
  else
    echo "  ✅ All .js files pass syntax parse check"
  fi
else
  echo "  ⚠  No parser available (node/tsc) — skipping parse check"
  WARNINGS=$((WARNINGS + 1))
fi
echo ""

# ── 3. Cyclomatic Complexity ──────────────────────────────────────
echo "─── 🔄 Cyclomatic Complexity ───"
case "$LANGUAGE" in
  python)
    if command -v radon &>/dev/null; then
      COMPLEX_OUTPUT=$(radon cc "$TARGET" --min C --show-complexity 2>/dev/null || true)
      if [ -n "$COMPLEX_OUTPUT" ]; then
        echo "$COMPLEX_OUTPUT"
        COMPLEX_COUNT=$(echo "$COMPLEX_OUTPUT" | grep -c " - [CDEF]$" || true)
        if [ "$COMPLEX_COUNT" -gt 0 ]; then
          echo "  ❌ $COMPLEX_COUNT function(s) exceed C-grade complexity"
          FAILED=$((FAILED + COMPLEX_COUNT))
        fi
      else
        echo "  ✅ All functions within complexity limits"
      fi
      AVG=$(radon cc "$TARGET" --average 2>/dev/null | grep -oP '[\d.]+(?= \(average)') || true
      if [ -n "$AVG" ]; then
        echo "  📊 Average cyclomatic complexity: $AVG"
        if [ "$(echo "$AVG > 5" | bc -l 2>/dev/null)" = "1" ]; then
          echo "  ⚠  Average above 5 — consider refactoring"
          WARNINGS=$((WARNINGS + 1))
        fi
      fi
    else
      echo "  ⚠  'radon' not installed. Install: pip install radon"
      WARNINGS=$((WARNINGS + 1))
    fi
    ;;

  typescript)
    if [ -f "node_modules/.bin/eslint" ]; then
      echo "  Checking with ESLint complexity rule..."
      npx eslint "$TARGET" --rule 'complexity/max-complexity: ["warn", 10]' --format compact 2>/dev/null || true
      # Count violations
      ES_COUNT=$(npx eslint "$TARGET" --rule 'complexity/max-complexity: ["error", 10]' --format compact 2>/dev/null | grep -c "complexity" || true)
      if [ "$ES_COUNT" -gt 0 ]; then
        echo "  ❌ $ES_COUNT function(s) exceed complexity threshold"
        FAILED=$((FAILED + ES_COUNT))
      else
        echo "  ✅ All functions within complexity limits (threshold: 10)"
      fi
    elif command -v eslint &>/dev/null; then
      echo "  ⚠  eslint found globally, but local install recommended"
      eslint "$TARGET" --rule 'complexity/max-complexity: ["warn", 10]' 2>/dev/null || echo "  (no issues found or eslint config missing)"
    else
      echo "  ⚠  eslint not found. Install: npm install --save-dev eslint"
      WARNINGS=$((WARNINGS + 1))
    fi
    ;;

  # .NET checks removed (not used in this project)

  *)
    echo "  ℹ  Complexity check not available for detected language"
    ;;
esac
echo ""

# ── 3. Code Style / Lint ──────────────────────────────────────────
echo "─── 🧹 Code Style & Lint ───"
case "$LANGUAGE" in
  python)
    if command -v flake8 &>/dev/null; then
      if LINT_OUTPUT=$(flake8 "$TARGET" --max-complexity=10 --max-line-length=100 2>&1); then
        echo "  ✅ No lint errors"
      else
        LINT_COUNT=$(echo "$LINT_OUTPUT" | wc -l)
        echo "$LINT_OUTPUT" | head -20
        [ "$LINT_COUNT" -gt 20 ] && echo "  ... and $((LINT_COUNT - 20)) more"
        echo "  ❌ $LINT_COUNT lint error(s)"
        FAILED=$((FAILED + LINT_COUNT))
      fi
    elif command -v ruff &>/dev/null; then
      if LINT_OUTPUT=$(ruff check "$TARGET" 2>&1); then
        echo "  ✅ No lint errors (ruff)"
      else
        LINT_COUNT=$(echo "$LINT_OUTPUT" | wc -l)
        echo "$LINT_OUTPUT" | head -20
        echo "  ❌ $LINT_COUNT lint error(s) (ruff)"
        FAILED=$((FAILED + LINT_COUNT))
      fi
    else
      echo "  ⚠  No linter found. Install: pip install flake8 (or ruff)"
      WARNINGS=$((WARNINGS + 1))
    fi
    ;;

  typescript)
    if [ -f "node_modules/.bin/eslint" ]; then
      if LINT_OUTPUT=$(npx eslint ".pi/extensions/" 2>&1); then
        echo "  ✅ No lint warnings"
      else
        ERROR_COUNT=$(echo "$LINT_OUTPUT" | grep -cE "error\s+" || true)
        WARN_COUNT=$(echo "$LINT_OUTPUT" | grep -cE "warning\s+" || true)
        echo "$LINT_OUTPUT" | head -30
        echo ""
        if [ "$ERROR_COUNT" -gt 0 ]; then
          echo "  ❌ $ERROR_COUNT lint error(s), $WARN_COUNT warning(s)"
          FAILED=$((FAILED + ERROR_COUNT))
        else
          echo "  ⚠  $WARN_COUNT lint warning(s) (advisory)"
          WARNINGS=$((WARNINGS + 1))
        fi
      fi
    else
      echo "  ⚠  eslint not found locally. Install: npm install --save-dev eslint"
      WARNINGS=$((WARNINGS + 1))
    fi
    ;;

  dotnet)
      echo "  ℹ  .NET checks not applicable to this project"
      ;;

  *)
    echo "  ℹ  Lint check not available for detected language"
    ;;
esac
echo ""

# ── 4. Tests & Coverage ───────────────────────────────────────────
echo "─── 🧪 Tests & Coverage ───"
case "$LANGUAGE" in
  python)
    if [ -d "tests" ] && command -v pytest &>/dev/null; then
      if COV_OUTPUT=$(pytest tests/ --cov="$TARGET" --cov-report=term --cov-fail-under=70 2>&1); then
        echo "$COV_OUTPUT" | tail -5
        echo "  ✅ Tests pass with >= 70% coverage"
      else
        echo "$COV_OUTPUT" | tail -10
        echo "  ❌ Tests or coverage below threshold"
        FAILED=$((FAILED + 1))
      fi
    elif ! command -v pytest &>/dev/null; then
      echo "  ⚠  pytest not installed. Install: pip install pytest pytest-cov"
      WARNINGS=$((WARNINGS + 1))
    else
      echo "  ⚠  No tests/ directory found"
    fi
    ;;

  typescript)
    # First: check that test files exist (TDD enforcement)
    TEST_FILES=$(find tests -name "*.test.*" -o -name "*.spec.*" 2>/dev/null | head -5)
    if [ -z "$TEST_FILES" ]; then
      echo "  ❌ No test files found in tests/ — TDD requires writing tests before implementation"
      FAILED=$((FAILED + 1))
    else
      echo "  ✅ Test files present: $(echo "$TEST_FILES" | wc -l) test file(s)"
    fi

    # Second: run tests if a runner is available
    if [ -f "node_modules/.bin/vitest" ]; then
      if VITEST_OUTPUT=$(npx vitest run --reporter=verbose 2>&1); then
        echo "$VITEST_OUTPUT" | tail -15
        echo "  ✅ All tests pass"
        PASSED=$((PASSED + 1))
      else
        echo "$VITEST_OUTPUT" | tail -20
        FAIL_COUNT=$(echo "$VITEST_OUTPUT" | grep -c "FAIL" || true)
        echo "  ❌ $FAIL_COUNT test(s) failed"
        FAILED=$((FAILED + 1))
      fi

      # Coverage check (non-blocking advisory)
      if [ -f "node_modules/.bin/@vitest/coverage-v8" ] || npm ls @vitest/coverage-v8 2>/dev/null | grep -q @vitest/coverage-v8; then
        if COV_OUTPUT=$(npx vitest run --coverage --coverage.thresholds.lines=70 2>&1); then
          echo "$COV_OUTPUT" | tail -5
          echo "  ✅ Coverage >= 70%"
        else
          echo "$COV_OUTPUT" | tail -10
          echo "  ⚠  Coverage below 70% threshold (advisory)"
          WARNINGS=$((WARNINGS + 1))
        fi
      fi
    elif [ -f "node_modules/.bin/jest" ]; then
      echo "  ⚠  Jest found — consider migrating to vitest for TypeScript-native support"
      npx jest --passWithNoTests 2>&1 | tail -5
    else
      echo "  ❌ No test runner found (vitest). Install: npm install --save-dev vitest"
      FAILED=$((FAILED + 1))
    fi
    ;;

  dotnet)
      echo "  ℹ  .NET checks not applicable to this project"
      ;;

  *)
    echo "  ℹ  Test run not available for detected language"
    ;;
esac
echo ""

# ── 5. Type Check (TypeScript-specific) ───────────────────────────
if [ "$LANGUAGE" = "typescript" ]; then
  echo "─── 🏷️  Type Check ───"
  if [ -f "node_modules/.bin/tsc" ]; then
    if TSC_OUTPUT=$(npx tsc --noEmit 2>&1); then
      echo "  ✅ TypeScript compiles without errors"
    else
      TSC_COUNT=$(echo "$TSC_OUTPUT" | grep -cE "error TS" || true)
      echo "$TSC_OUTPUT" | head -15
      echo "  ❌ $TSC_COUNT type error(s)"
      FAILED=$((FAILED + TSC_COUNT))
    fi
  else
    echo "  ⚠  tsc not found. Install: npm install --save-dev typescript"
    WARNINGS=$((WARNINGS + 1))
  fi
  echo ""
fi

# ── Summary ───────────────────────────────────────────────────────
echo "═══════════════════════════════════════════════════════════════"
echo "  Results: ● $PASSED passed  ✗ $FAILED failed  ⚠ $WARNINGS warnings"
echo "═══════════════════════════════════════════════════════════════"

if [ "$FAILED" -gt 0 ]; then
  echo ""
  echo "  ❌ Quality Gate FAILED — review issues above before declaring done"
  exit 1
elif [ "$WARNINGS" -gt 0 ]; then
  echo ""
  echo "  ⚠  Quality Gate PASSED with warnings — address when practical"
  exit 0
else
  echo ""
  echo "  ✅ Quality Gate PASSED — code quality is within thresholds"
  exit 0
fi
