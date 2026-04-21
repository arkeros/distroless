#!/usr/bin/env bash

set -o errexit -o nounset -o pipefail

git_commit=$(git rev-parse HEAD)
readonly git_commit

# Monorepo version. For example, 2025.34.0+201b9a8.
# Follows https://blog.aspect.build/versioning-releases-from-a-monorepo
monorepo_version=$(
    git describe --tags --long --match="2[0-9][0-9][0-9].[1-9]" --match="2[0-9][0-9][0-9].[1-5][0-9]" 2>/dev/null |
        sed -e 's/-/./;s/-g/+/' || echo "0.0.0+${git_commit:0:8}"
)

# Short version without build metadata. For example, 2025.34.0.
monorepo_short_version="${monorepo_version%%+*}"

# Image repository compatible monorepo version. For example, 2025.34.0-201b9a8.
# OCI registries do not allow `+` characters in tags so we swap with `-`.
monorepo_image_tag_version="${monorepo_version//+/-}"

cat <<EOF
STABLE_GIT_COMMIT ${git_commit}
STABLE_GIT_SHORT_COMMIT ${git_commit:0:8}
STABLE_MONOREPO_VERSION ${monorepo_version}
STABLE_MONOREPO_SHORT_VERSION ${monorepo_short_version}
STABLE_MONOREPO_IMAGE_TAG_VERSION ${monorepo_image_tag_version}
EOF
