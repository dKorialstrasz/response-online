#!/usr/bin/env bash
set -euo pipefail

git diff --check

required_files=(
  CONTRIBUTING.md
  docs/requirements/milestone-1.md
  docs/requirements/milestone-1-roadmap.md
  docs/decisions/m1-product-decisions.md
  docs/adr/0001-in-memory-match-state.md
)

for required_file in "${required_files[@]}"; do
  test -s "${required_file}"
done

if rg -n 'Heartbeat каждые 10 секунд|120/300 секунд reconnect|30 минут технической неактивности|EN fallback.*обязательны|минимум одного независимого review|AC-01–12' CONTRIBUTING.md docs; then
  echo 'Обнаружено устаревшее решение M1.' >&2
  exit 1
fi

rg -q 'heartbeat — 1 раз/с' docs/decisions/m1-product-decisions.md
rg -q 'техническое завершение через 90 с' docs/decisions/m1-product-decisions.md
rg -q 'GamePix полностью исключён из M1' docs/decisions/m1-product-decisions.md
rg -q 'только на русском языке' docs/decisions/m1-product-decisions.md

echo 'Docs checks passed.'
