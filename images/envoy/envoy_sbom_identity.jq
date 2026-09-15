# How envoy is identified in an index SBOM, for the diff against
# envoy_lock_version.jq's reading of the lockfile (BUILD). The same filter
# reads a consumer's scan of the image, which is the point: the two views
# have to agree.
#
# `versions` lists every upstream version an envoy component carries as
# `pkg:generic/envoy@<version>` *and* as the matching envoyproxy:envoy CPE —
# a component missing either drops out, and the diff fails. `distro_envoy`
# counts envoy components under a deb or rpm purl, which would route grype to
# a distro tracker for a build no distro made; it must be zero.
{
  versions: (
    [
      .components[]
      | select(.name == "envoy")
      | ((.purl // "") | capture("^pkg:generic/envoy@(?<v>[^?]+)") | .v) as $v
      | select(.cpe == "cpe:2.3:a:envoyproxy:envoy:\($v):*:*:*:*:*:*:*")
      | $v
    ]
    | unique
  ),
  distro_envoy: ([.components[] | select((.purl // "") | test("^pkg:(deb|rpm)/[^/]+/envoy@"))] | length),
}
