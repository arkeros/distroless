"""Bazel rules that mirror busybox's kbuild conventions.

Compiling a busybox `.c` file requires:
- `-include include/autoconf.h` so every translation unit sees the resolved kconfig
- A handful of feature-test macros (`_GNU_SOURCE`, `_FILE_OFFSET_BITS=64`, ...)
- busybox-specific warning suppressions for code patterns clang flags

`kbuild_lib` wraps `cc_library` with these defaults so the per-subdir BUILD
declarations stay compact (just `srcs = [...]` plus optional `deps`).
"""

load("@rules_cc//cc:defs.bzl", "cc_library")

# These mirror the flags busybox kbuild assembles in Makefile.flags after
# applying the resolved kconfig. We omit:
# - `-finline-limit=0`, `-falign-jumps=1`, `-falign-labels=1` — gcc-only
# - `-fpie` — Bazel's toolchain adds it for us
# - `-DKBUILD_BASENAME='"<file>"'` / `-DKBUILD_MODNAME='"<file>"'` — per-file
#   trace macros that busybox never reads back; safe to drop.
KBUILD_COPTS = [
    "-std=gnu99",
    "-include",
    "include/autoconf.h",
    "-Os",
    "-fomit-frame-pointer",
    "-ffunction-sections",
    "-fdata-sections",
    "-funsigned-char",
    "-fno-builtin-strlen",
    "-fno-builtin-printf",
    "-fno-unwind-tables",
    "-fno-asynchronous-unwind-tables",
    "-Wall",
    "-Wshadow",
    "-Wwrite-strings",
    "-Wundef",
    "-Wstrict-prototypes",
    "-Wno-format-security",
    "-Wdeclaration-after-statement",
    "-Wold-style-definition",
    # kbuild emits gcc-only opt flags; let clang ignore quietly.
    "-Wno-ignored-optimization-argument",
    "-Wno-unknown-warning-option",
    # busybox source style intentionally trips these.
    "-Wno-string-plus-int",
    "-Wno-self-assign",
    "-Wno-misleading-indentation",
    "-Wno-pointer-sign",
    "-Wno-unused-parameter",
]

KBUILD_LOCAL_DEFINES = [
    "_GNU_SOURCE",
    "NDEBUG",
    "_LARGEFILE_SOURCE",
    "_LARGEFILE64_SOURCE",
    "_FILE_OFFSET_BITS=64",
    "_TIME_BITS=64",
    "BB_VER=\\\"1.37.0\\\"",
]

def kbuild_lib(name, srcs, textual_hdrs = [], deps = [], copts = [], local_defines = []):
    """A static cc_library compiled with busybox kbuild's standard flags.

    Use one per kbuild subdir, mirroring upstream's per-subdir `built-in.o`s.

    `textual_hdrs` is for the rare busybox files that `#include "sibling.c"`
    inline (e.g. `coreutils/od.c` includes `od_bloaty.c`; sandboxed compiles
    need the included file declared as an input even though it's not a TU).
    Pass the sibling files here and `exclude` them from `srcs` — see
    //coreutils/BUILD.bazel for an example. Only 3 subpackages need this.
    """
    cc_library(
        name = name,
        srcs = srcs,
        # Standard cc_library pattern: glob the package's `.h` files
        # (recursive — `networking/libiproute/*.h` etc. live inside their
        # parent kbuild package, not a separate one). Sandboxed compiles
        # need these declared as inputs even for `#include "sibling.h"`.
        hdrs = native.glob(["**/*.h"], allow_empty = True),
        textual_hdrs = textual_hdrs,
        deps = deps + ["//:kbuild_headers"],
        copts = KBUILD_COPTS + copts,
        local_defines = KBUILD_LOCAL_DEFINES + local_defines,
        linkstatic = True,
        # busybox applets register via static constructors / linker-section
        # tricks; keep every .o referenced from the binary.
        alwayslink = True,
        # libbb.h drags in <shadow.h>, <sys/mount.h>, etc. — Linux-only.
        # Skips the target on non-Linux hosts when invoked via `//...`.
        target_compatible_with = ["@platforms//os:linux"],
    )
