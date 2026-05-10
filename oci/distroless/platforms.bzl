"Shared distroless distro metadata and platform mappings."

VERSIONS = [
    # (distro_key, codename, version_id) — distro_key matches the apt.install
    # name in MODULE.bazel; codename + version_id flow into /etc/os-release.
    # `version_id` must be parseable per os-release(5) — no slashes, no spaces.
    # We track Debian unstable (sid), so version_id is "unstable": that's the
    # value grype's debian distro normaliser maps to the sid CVE feed
    # (debian:sid is *not* recognised, debian:unstable is). An earlier "13/sid"
    # was both spec-non-compliant (slash) and silently broke vuln scanning by
    # parsing as neither debian:13 nor debian:unstable, leaving every Debian
    # matcher dormant and producing misleading 0-vuln output.
    ("debian", "sid", "unstable"),
]

VARIANTS = {
    "arm": "v7",
    "arm64": "v8",
}

ARCHITECTURE_PLATFORMS = {
    "amd64": "//bazel/platforms:linux_amd64",
    "arm64": "//bazel/platforms:linux_arm64",
}

ALL_ARCHITECTURES = ["amd64", "arm64"]
ALL_DISTROS = ["debian"]
