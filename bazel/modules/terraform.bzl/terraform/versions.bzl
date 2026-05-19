"""Terraform version definitions with archive filenames and checksums.

Binaries come from HashiCorp's official releases at
`https://releases.hashicorp.com/terraform/{version}/{archive}`. Each
archive is a `.zip` containing a single `terraform` executable.

Format: "VERSION-PLATFORM": (filename, sha256)
Platforms: darwin_amd64, darwin_arm64, linux_amd64, linux_arm64

To bump:
  1. Pick a new version on https://releases.hashicorp.com/terraform/.
  2. Read the SHA256SUMS for that version, copy the four entries that
     match `terraform_<version>_{darwin,linux}_{amd64,arm64}.zip`.
  3. Append a new block here and update DEFAULT_VERSION.
"""

DEFAULT_VERSION = "1.14.8"

TERRAFORM_VERSIONS = {
    "1.14.8-darwin_amd64": (
        "terraform_1.14.8_darwin_amd64.zip",
        "26dd7593d22e9d99ec09380f0869718f649be7b7f954d888611335e6a84961f8",
    ),
    "1.14.8-darwin_arm64": (
        "terraform_1.14.8_darwin_arm64.zip",
        "5593670a2d42323847bfb216db17c73a44df201a62f7587928bae16adeabba23",
    ),
    "1.14.8-linux_amd64": (
        "terraform_1.14.8_linux_amd64.zip",
        "56a5d12f47cbc1c6bedb8f5426ae7d5df984d1929572c24b56f4c82e9f9bf709",
    ),
    "1.14.8-linux_arm64": (
        "terraform_1.14.8_linux_arm64.zip",
        "c953171cde6b25ca0448c3b29a90d2f46c0310121e18742ec8f89631768e770c",
    ),
}

def get_terraform_url(version, filename):
    """Returns the download URL for a terraform release archive."""
    return "https://releases.hashicorp.com/terraform/{}/{}".format(version, filename)
