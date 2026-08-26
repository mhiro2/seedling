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

# A stable seedlingpgx release must build on a stable parent, so prereleases are
# never candidates. The tag under release is excluded because it does not exist
# yet from the perspective of the module graph.
# Enumerating the remote is the only step that can fail for reasons unrelated to
# the invariant, so its exit status is checked before anything reads the result.
if ! remote_refs="$(git ls-remote --tags origin 'refs/tags/v*')"; then
  echo "Failed to list ${parent_module} tags from origin" >&2
  exit 1
fi

latest=""
while read -r tag; do
  if [[ "$tag" != "$release_tag" ]]; then
    latest="$tag"
  fi
done < <(
  printf '%s\n' "$remote_refs" |
    sed -e 's|^.*refs/tags/||' -e 's|\^{}$||' |
    grep -E "$stable_version_pattern" |
    sort -V -u
)

if [[ -z "$latest" ]]; then
  echo "No stable ${parent_module} release precedes ${release_tag:-this check}, so there is nothing to track yet"
  exit 0
fi

if [[ "$required" != "$latest" ]]; then
  echo "${module_go_mod} requires ${parent_module} ${required}, expected ${latest}" >&2
  echo "Run: (cd seedlingpgx && GOWORK=off go get ${parent_module}@${latest} && GOWORK=off go mod tidy)" >&2
  exit 1
fi

echo "${module_go_mod} requires ${parent_module} ${required}, matching the latest published tag"
