"Shared distroless distro metadata and platform mappings."

VERSIONS = [
    # (distro_key, codename, version_id) — distro_key matches the apt.install
    # name in MODULE.bazel; codename + version_id flow into /etc/os-release.
    # "13/sid" matches the PRETTY_NAME convention real Debian sid systems use
    # ("Debian GNU/Linux trixie/sid") — sid is the in-flight feed between
    # released stable versions, so it has no clean numeric version_id.
    ("debian", "sid", "13/sid"),
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
