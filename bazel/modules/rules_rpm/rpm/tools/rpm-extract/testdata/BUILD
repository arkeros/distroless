package(default_visibility = ["//rpm/tools/rpm-extract:__pkg__"])

# Real Hummingbird tzdata-2026a-1.1.hum1.noarch.rpm fetched from
# https://koji-s3-cache.hummingbird-project.io/.../x86_64/Packages/t/tzdata-2026a-1.1.hum1.noarch.rpm
# sha256 af704da5d79cb20c1d36a7a170ea008db82001433d4a710cd37fdbb4def0bdb9
# Refresh by re-fetching with `curl -fsSL` if the upstream revision moves.
#
# hummingbird-release.pgp is a verbatim copy of the repo-root file consumed
# by @hummingbird//... — duplicated here because this module is vendored
# and must stay self-contained for `bazel test @rules_rpm//...` to work
# without depending on the parent repo's layout.
exports_files([
    "hummingbird-release.pgp",
    "tzdata.rpm",
])
