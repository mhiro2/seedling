#!/usr/bin/env bash

set -euo pipefail

readonly parent_module="github.com/mhiro2/seedling"
readonly module_go_mod="seedlingpgx/go.mod"

if [[ $# -gt 1 ]]; then
  echo "Usage: $0 [vX.Y.Z]" >&2
  exit 2
fi

readonly release_tag="${1:-}"
readonly numeric_identifier='(0|[1-9][0-9]*)'
readonly prerelease_identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
readonly stable_version_pattern="^v${numeric_identifier}\.${numeric_identifier}\.${numeric_identifier}$"
readonly version_pattern="^v${numeric_identifier}\.${numeric_identifier}\.${numeric_identifier}(-${prerelease_identifier}(\.${prerelease_identifier})*)?$"
if [[ -n "$release_tag" && ! "$release_tag" =~ $version_pattern ]]; then
  echo "Version must be a valid semantic version beginning with v: $release_tag" >&2
  exit 2
fi

required="$(
  awk -v module="$parent_module" '
    $1 == "require" && $2 == module { print $3; exit }
    $1 == "require" && $2 == "(" { in_block = 1; next }
    in_block && $1 == ")" { in_block = 0; next }
    in_block && $1 == module { print $2; exit }
  ' "$module_go_mod"
)"
if [[ -z "$required" ]]; then
  echo "${module_go_mod} does not require ${parent_module}" >&2
  exit 1
fi

# The replace directive points seedlingpgx at the in-repo parent, so the
# requirement can name a release that is not tagged yet. Consumers ignore it and
# resolve the required version instead.
replacement="$(
  awk -v module="$parent_module" '
    $1 == "replace" && $2 == module && $3 == "=>" { print $4; exit }
    $1 == "replace" && $2 == "(" { in_block = 1; next }
    in_block && $1 == ")" { in_block = 0; next }
    in_block && $1 == module && $2 == "=>" { print $3; exit }
  ' "$module_go_mod"
)"
if [[ "$replacement" != "../" && "$replacement" != ".." ]]; then
  echo "${module_go_mod} must replace ${parent_module} with the in-repo module (../)" >&2
  exit 1
fi

readonly update_hint="Run: (cd seedlingpgx && GOWORK=off go mod edit -require=${parent_module}@<version> && GOWORK=off go mod tidy)"

# seedlingpgx/vX.Y.Z is tagged on the same commit as vX.Y.Z, so the release
# must require exactly the parent version it ships alongside.
if [[ -n "$release_tag" ]]; then
  if [[ "$required" != "$release_tag" ]]; then
    echo "${module_go_mod} requires ${parent_module} ${required}, expected ${release_tag}" >&2
    echo "${update_hint/<version>/${release_tag}}" >&2
    exit 1
  fi
  echo "${module_go_mod} requires ${parent_module} ${required}, matching the release tag"
  exit 0
fi

# Without a release tag, the requirement names either the latest release or the
# one being prepared, and must never fall behind what is already published.
if ! remote_refs="$(git ls-remote --tags origin 'refs/tags/v*')"; then
  echo "Failed to list ${parent_module} tags from origin" >&2
  exit 1
fi

latest="$(
  printf '%s\n' "$remote_refs" |
    sed -e 's|^.*refs/tags/||' -e 's|\^{}$||' |
    grep -E "$stable_version_pattern" |
    sort -V -u |
    tail -n 1 ||
    true
)"

if [[ -z "$latest" ]]; then
  echo "No stable ${parent_module} release is published yet, so there is nothing to compare against"
  exit 0
fi

# sort -V orders vX.Y.Z before vX.Y.Z-rc.N, so prereleases are compared by
# their core version and count as behind a stable release of that version.
required_core="${required%%-*}"
if [[ "$(printf '%s\n%s\n' "$latest" "$required_core" | sort -V | head -n 1)" != "$latest" ||
  ("$required_core" == "$latest" && "$required" != "$latest") ]]; then
  echo "${module_go_mod} requires ${parent_module} ${required}, older than the latest published ${latest}" >&2
  echo "${update_hint}" >&2
  exit 1
fi

echo "${module_go_mod} requires ${parent_module} ${required}, not behind the latest published ${latest}"
