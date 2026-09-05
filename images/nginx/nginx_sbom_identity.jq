# How the nginx.org package is identified in an index SBOM, for the diff
# against nginx_lock_version.jq's reading of the lockfile (BUILD).
#
# `versions` lists every upstream version an nginx component carries as
# `pkg:generic/nginx@<version>` *and* as the matching f5:nginx CPE — a
# component missing either drops out, and the diff fails. `distro_nginx`
# counts nginx components still under a deb or rpm purl, which would route
# grype to Debian's or Hummingbird's tracker for a build neither of them made;
# it must be zero. See `upstream_identities` in //bazel/include/oci.MODULE.bazel.
{
  versions: (
    [
      .components[]
      | select(.name == "nginx")
      | ((.purl // "") | capture("^pkg:generic/nginx@(?<v>[^?]+)") | .v) as $v
      | select(.cpe == "cpe:2.3:a:f5:nginx:\($v):*:*:*:*:*:*:*")
      | $v
    ]
    | unique
  ),
  distro_nginx: ([.components[] | select((.purl // "") | test("^pkg:(deb|rpm)/[^/]+/nginx@"))] | length),
}
