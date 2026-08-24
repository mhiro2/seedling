#!/usr/bin/env bash

set -euo pipefail

readonly module_path="github.com/mhiro2/seedling/seedlingpgx"

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 vX.Y.Z[-PRERELEASE]" >&2
  exit 2
fi

readonly version="$1"
readonly numeric_identifier='(0|[1-9][0-9]*)'
readonly prerelease_identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
readonly version_pattern="^v${numeric_identifier}\.${numeric_identifier}\.${numeric_identifier}(-${prerelease_identifier}(\.${prerelease_identifier})*)?$"
if [[ ! "$version" =~ $version_pattern ]]; then
  echo "Version must be a valid semantic version beginning with v: $version" >&2
  exit 2
fi

consumer_dir="$(mktemp -d)"
trap 'rm -rf -- "${consumer_dir:?}"' EXIT

cd "$consumer_dir"
export GOWORK=off

go mod init example.com/seedlingpgx-consumer

readonly max_attempts=5
attempt=1
until go get "${module_path}@${version}"; do
  if ((attempt == max_attempts)); then
    echo "Failed to resolve ${module_path}@${version} after ${max_attempts} attempts" >&2
    exit 1
  fi

  delay=$((attempt * 5))
  echo "Module is not available yet; retrying in ${delay} seconds (${attempt}/${max_attempts})" >&2
  sleep "$delay"
  attempt=$((attempt + 1))
done

cat >consumer_test.go <<'EOF'
package consumer_test

import (
	"testing"

	"github.com/mhiro2/seedling/seedlingpgx"
)

func TestSeedlingPGXImport(_ *testing.T) {
	var _ seedlingpgx.Beginner
}
EOF

if grep -Eq '^[[:space:]]*replace([[:space:](]|$)' go.mod; then
  echo "Consumer go.mod unexpectedly contains a replace directive" >&2
  exit 1
fi

go test ./...
