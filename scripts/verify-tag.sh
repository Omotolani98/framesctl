#!/bin/sh
set -eu

tag="${1:-${GITHUB_REF_NAME:-}}"
commit="${2:-${GITHUB_SHA:-HEAD}}"
main_ref="refs/remotes/origin/main"

if [ -z "$tag" ]; then
  echo "release tag is required" >&2
  exit 1
fi

if ! printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'; then
  echo "tag must be SemVer like v1.2.3" >&2
  exit 1
fi

git fetch --no-tags origin "+refs/heads/main:${main_ref}"

tag_commit="$(git rev-parse "${commit}^{commit}")"

if ! git merge-base --is-ancestor "$tag_commit" "$main_ref"; then
  echo "refusing release: tagged commit is not on main" >&2
  exit 1
fi

echo "release tag $tag is valid and on main"
