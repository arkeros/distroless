# The upstream nginx version as a lockfile records it, in the shape
# nginx_sbom_identity.jq reads from the SBOM, so the two can be diffed. An
# apt lockfile lists packages as an array (`1.30.4-1~trixie`); an rpm
# lockfile keys them by name, then architecture (`2:1.30.4-1.el10.ngx`).
# Epoch and distro revision are stripped, as the package extensions strip
# them when they emit the identity (`upstream_identities` on apt.install and
# rpm.install, //bazel/include/oci.MODULE.bazel).
def upstream_version:
  sub("^[0-9]+:"; "") | sub("-[^-]*$"; "");

{
  versions: (
    [
      (.packages | if type == "array" then .[] | select(.name == "nginx") else .nginx[] end)
      | .version
      | upstream_version
    ]
    | unique
  ),
  distro_nginx: 0,
}
