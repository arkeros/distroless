"common variables"

def quote(str):
    return '''"{}"'''.format(str)

OS_RELEASE = dict(
    PRETTY_NAME = "Distroless",
    NAME = "Debian GNU/Linux",
    ID = "debian",
    VERSION_ID = "{VERSION}",
    VERSION = "Debian GNU/Linux {VERSION} ({CODENAME})",
    HOME_URL = "https://github.com/arkeros/astrograde",
    SUPPORT_URL = "https://github.com/arkeros/astrograde/blob/main/distroless/README.md",
    BUG_REPORT_URL = "https://github.com/arkeros/astrograde/issues/new",
)

NOBODY = 65534
NONROOT = 65532
ROOT = 0

USER_IDS = {
    "root": ROOT,
    "nonroot": NONROOT,
}

MTIME = "0"

DEBUG_MODE = ["", "_debug"]
USERS = ["root", "nonroot"]

COMPRESSION = "zstd"
