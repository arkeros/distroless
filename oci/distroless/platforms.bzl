"Shared distroless distro metadata and platform mappings."

VERSIONS = [
    # ("debian12", "bookworm", "12"),
    ("debian13", "trixie", "13"),
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
ALL_DISTROS = ["debian13"]
