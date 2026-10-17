#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${GITHUB_ACTIONS:-}" ]] && git rev-parse --verify HEAD^1 >/dev/null 2>&1; then
  git diff --check HEAD^1 HEAD
else
  git diff --check
fi

search_regex() {
  if command -v rg >/dev/null 2>&1; then
    rg -n "$@"
  else
    local pattern="$1"
    shift
    grep -nE "$pattern" "$@"
  fi
}

contains_text() {
  local pattern="$1"
  local file="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -q "$pattern" "$file"
  else
    grep -qF "$pattern" "$file"
  fi
}

required_files=(
  CONTRIBUTING.md
  docs/requirements/milestone-1.md
  docs/requirements/milestone-1-roadmap.md
  docs/decisions/m1-product-decisions.md
  docs/adr/0001-in-memory-match-state.md
  docs/adr/0002-modular-monolith-boundaries.md
  docs/adr/0003-serialized-match-execution.md
  docs/adr/0004-protocol-and-player-views.md
  docs/adr/README.md
  docs/runbooks/local-development.md
)

for required_file in "${required_files[@]}"; do
  test -s "${required_file}"
done

if search_regex 'Heartbeat каждые 10 секунд|120/300 секунд reconnect|30 минут технической неактивности|EN fallback.*обязательны|минимум одного независимого review|AC-01–12' CONTRIBUTING.md docs; then
  echo 'Обнаружено устаревшее решение M1.' >&2
  exit 1
fi

contains_text 'heartbeat — 1 раз/с' docs/decisions/m1-product-decisions.md
contains_text 'техническое завершение через 90 с' docs/decisions/m1-product-decisions.md
contains_text 'GamePix полностью исключён из M1' docs/decisions/m1-product-decisions.md
contains_text 'только на русском языке' docs/decisions/m1-product-decisions.md

python3 scripts/validate_decks.py
python3 scripts/validate_contracts.py

echo 'Docs checks passed.'
