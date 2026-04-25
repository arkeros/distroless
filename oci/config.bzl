# rules_img requires registry and repository as separate values
OCI_REGISTRY = "ghcr.io"
OCI_REPOSITORY_PREFIX = "arkeros/senku"

# GAR destination, provisioned by //infra/cloud/gcp/gar. Cloud Run pulls from
# here; GHCR stays as the public-facing mirror for `distroless.io`.
GAR_REGISTRY = "europe-docker.pkg.dev"
GAR_REPOSITORY_PREFIX = "senku-prod/containers"

GO_DISTROS = ["debian13"]
GO_ARCHITECTURES = {
    "debian13": ["amd64", "arm64"],
}

PYTHON_DISTROS = ["debian13"]
PYTHON_ARCHITECTURES = {
    "debian13": ["amd64", "arm64"],
}

PYTHON_PACKAGES = [
    "libbz2-1.0",
    # "libdb5.3",
    "libexpat1",
    "liblzma5",
    "libsqlite3-0",
    "libuuid1",
    "libncursesw6",
    "libtinfo6",
    "zlib1g",
    "libcom-err2",
    "libcrypt1",
    "libgssapi-krb5-2",
    "libk5crypto3",
    "libkeyutils1",
    "libkrb5-3",
    "libkrb5support0",
    "libnsl2",
    # "libreadline8",
    # "libtirpc3",
    "libffi8",
]

NODEJS_DISTROS = ["debian13"]
NODEJS_ARCHITECTURES = {
    "debian13": ["amd64", "arm64"],
}
