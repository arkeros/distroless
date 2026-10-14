# The envoy version the lockfile pins for one line, in the shape
# envoy_sbom_identity.jq reads from an SBOM, so the two can be diffed. The
# line comes in as `$line` (`--arg line 1.39` in BUILD), since one lockfile
# holds every line this family ships.
{
  versions: [.lines[] | select(.major == $line) | .version],
  distro_envoy: 0,
}
