"""Cosign version definitions with URLs and checksums.

Refresh by re-fetching:

    curl -fsSL https://api.github.com/repos/sigstore/cosign/releases/latest | jq -r .tag_name
    curl -fsSL https://github.com/sigstore/cosign/releases/download/v<VERSION>/cosign_checksums.txt
"""

DEFAULT_VERSION = "3.0.6"

# Format: "VERSION-PLATFORM": (filename, sha256)
# Platforms: darwin_amd64, darwin_arm64, linux_amd64, linux_arm64
COSIGN_VERSIONS = {
    "3.0.6-darwin_amd64": (
        "cosign-darwin-amd64",
        "4c3e7af8372d3ca3296e62fa56f23fcbb5721cc6ac1827900d398f110d7cd280",
    ),
    "3.0.6-darwin_arm64": (
        "cosign-darwin-arm64",
        "5fadd012ae6381a6a29ff86a7d39aa873878852f1073fc90b15995961ecfb084",
    ),
    "3.0.6-linux_amd64": (
        "cosign-linux-amd64",
        "c956e5dfcac53d52bcf058360d579472f0c1d2d9b69f55209e256fe7783f4c74",
    ),
    "3.0.6-linux_arm64": (
        "cosign-linux-arm64",
        "bedac92e8c3729864e13d4a17048007cfafa79d5deca993a43a90ffe018ef2b8",
    ),
}

def get_cosign_url(version, filename):
    """Returns the download URL for a cosign release."""
    return "https://github.com/sigstore/cosign/releases/download/v{}/{}".format(version, filename)
