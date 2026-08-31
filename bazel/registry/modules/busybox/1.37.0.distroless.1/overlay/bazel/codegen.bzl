"""Bazel-native codegen for busybox's kbuild-derived headers.

Every header that used to live under //busybox/generated/ is produced here
from `busyboxconfig` + @busybox_src. Three host `cc_binary` targets back the
non-shell generators:

  - :kconfig_conf_host   — busybox's silent oldconfig front-end, used to
                            translate .config into include/autoconf.h
  - :applet_tables_host  — emits include/applet_tables.h + NUM_APPLETS.h
  - :usage_host          — prints the packed usage strings consumed by
                            applets/usage_compressed to produce
                            include/usage_compressed.h

The genrules wire those tools to the shell scripts shipped in @busybox_src
(`scripts/mkconfigs`, `scripts/generate_BUFSIZ.sh`, `scripts/embedded_scripts`,
`scripts/gen_build_files.sh`, `applets/usage_compressed`) and stage them in a
private workdir that mirrors a busybox source tree with `.config` at the
root, since the upstream scripts assume that layout.
"""

load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library")

# Generated headers, in dependency order. Anything in this list is produced
# under the `include/` subdir of //busybox at build time.
GENERATED_HEADERS = [
    "include/bbconfigopts.h",
    "include/bbconfigopts_bz2.h",
    "include/common_bufsiz.h",
    "include/embedded_scripts.h",
    "include/applets.h",
    "include/usage.h",
    "include/autoconf.h",
    "include/applet_tables.h",
    "include/NUM_APPLETS.h",
    "include/usage_compressed.h",
]

# Host-built `cc_binary`s that the genrules invoke as `tools`. The kconfig
# front-end pulls in flex/bison output via the upstream `_shipped` files so we
# don't take a build-tool dep on flex/bison.
_KCONFIG_RENAMED = {
    "scripts/kconfig/zconf.tab.c": "//:scripts/kconfig/zconf.tab.c_shipped",
    "scripts/kconfig/lex.zconf.c": "//:scripts/kconfig/lex.zconf.c_shipped",
    "scripts/kconfig/zconf.hash.c": "//:scripts/kconfig/zconf.hash.c_shipped",
}

def _rename_shipped_sources():
    """Genrules that copy `*.c_shipped` into the names their textual includers expect."""
    for out, src in _KCONFIG_RENAMED.items():
        native.genrule(
            name = out.replace("/", "_").replace(".", "_"),
            srcs = [src],
            outs = [out],
            cmd = "cp $< $@",
        )

# Sed expression that prepends `$$PWD/` to every exec-root-relative tool path
# we pass to a script that has cd'd into a workdir. Used by tools_to_abs.
_TOOL_ABS_SED = "s#^#$$PWD/#"

def _stage_tree_and_run(name, srcs, tools, outs, cmd_body):
    """Common scaffold: copy @busybox_src + .config into a writable workdir,
    cd into it, run the provided shell snippet, then copy outputs back out.

    The snippet runs with `$$WORK` set to the workdir root and the listed
    `tools` available via $(execpath ...) using ABSOLUTE paths (the workdir
    is outside the exec root, so relative paths wouldn't resolve).
    """
    native.genrule(
        name = name,
        srcs = srcs + [
            "//:busyboxconfig",
            "//:srcs",
        ],
        outs = outs,
        tools = tools,
        cmd = """\
set -eu
EXEC_ROOT="$$PWD"
RULEDIR_ABS="$$EXEC_ROOT/$(RULEDIR)"
WORK=$$(mktemp -d "$${{TMPDIR:-/tmp}}/bb-codegen-{name}.XXXXXX")
trap "chmod -R u+w $$WORK 2>/dev/null; rm -rf $$WORK" EXIT

# Materialize a writable copy of the busybox source tree.
SRC_ROOT=$$(dirname $$(echo $(execpaths //:srcs) | tr ' ' '\\n' | grep '/Makefile$$' | head -1))
cp -RL "$$EXEC_ROOT/$$SRC_ROOT/." "$$WORK/"
chmod -R u+w "$$WORK"

# Drop the resolved .config at the source root; every codegen script reads it
# from `.config` in cwd.
cp $(execpath //:busyboxconfig) "$$WORK/.config"

cd "$$WORK"
{cmd_body}

# Copy declared outputs back to bazel-out (paths are exec-root-relative).
mkdir -p "$$RULEDIR_ABS/include"
for f in {outs_basenames}; do
  cp "$$WORK/include/$$f" "$$RULEDIR_ABS/include/$$f"
done
""".format(
            name = name,
            cmd_body = cmd_body,
            outs_basenames = " ".join([o.split("/")[-1] for o in outs]),
        ),
    )

def busybox_codegen():
    """Emit every kbuild-derived header as a Bazel rule.

    Call once from //busybox:BUILD.bazel. Produces targets named after each
    output file (with `/` → `_`).
    """

    _rename_shipped_sources()

    # ─── Host build tools ────────────────────────────────────────────────────
    #
    # All three tools compile on the build host (macOS, Linux, ...). They run
    # during codegen to emit headers consumed by the target compile, so they
    # must always use the exec-platform toolchain — genrules wire them in via
    # `tools = [...]` which forces that automatically.

    # kconfig's silent-oldconfig binary.
    cc_binary(
        name = "kconfig_conf_host",
        srcs = [
            "//:scripts/kconfig/conf.c",
            ":scripts/kconfig/zconf.tab.c",
        ],
        # busybox kconfig has many gcc-isms; not our code to fix.
        copts = ["-w"],
        # KBUILD_NO_NLS skips the libintl/gettext dependency; we just need
        # the parser, not localization.
        local_defines = ["KBUILD_NO_NLS"],
        # The include path for the textually-included siblings comes via
        # `includes = ["scripts/kconfig"]` on :kconfig_conf_host_textual_deps;
        # Bazel rewrites it to the correct `external/<repo>/...` prefix so we
        # don't have to hardcode bzlmod's canonical repo name.
        deps = [":kconfig_conf_host_textual_deps"],
        target_compatible_with = [],
        visibility = ["//visibility:private"],
    )

    # Textually-included files for zconf.tab.c.
    cc_library(
        name = "kconfig_conf_host_textual_deps",
        textual_hdrs = [
            "//:scripts/kconfig/confdata.c",
            "//:scripts/kconfig/expr.c",
            "//:scripts/kconfig/menu.c",
            "//:scripts/kconfig/symbol.c",
            "//:scripts/kconfig/util.c",
            ":scripts/kconfig/lex.zconf.c",
            ":scripts/kconfig/zconf.hash.c",
        ],
        hdrs = [
            "//:scripts/kconfig/lkc.h",
            "//:scripts/kconfig/lkc_proto.h",
            "//:scripts/kconfig/expr.h",
        ],
        includes = ["scripts/kconfig"],
    )

    # applet_tables.c uses `#include "../include/autoconf.h"` — the path
    # arithmetic is relative to the source file's *physical* location. Stage
    # both host-tool .c files and the headers they include under a single
    # `host_layout/` directory tree so the upstream-style include works
    # without source modification.
    native.genrule(
        name = "host_layout",
        srcs = [
            "//applets:applet_tables.c",
            "//applets:usage.c",
            "//:include/applet_metadata.h",
            ":include/autoconf.h",
            ":include/applets.h",
            ":include/usage.h",
        ],
        outs = [
            "host_layout/applets/applet_tables.c",
            "host_layout/applets/usage.c",
            "host_layout/include/applet_metadata.h",
            "host_layout/include/autoconf.h",
            "host_layout/include/applets.h",
            "host_layout/include/usage.h",
        ],
        cmd = """\
mkdir -p $(RULEDIR)/host_layout/applets $(RULEDIR)/host_layout/include
cp $(execpath //applets:applet_tables.c) $(RULEDIR)/host_layout/applets/applet_tables.c
cp $(execpath //applets:usage.c)          $(RULEDIR)/host_layout/applets/usage.c
cp $(execpath //:include/applet_metadata.h) $(RULEDIR)/host_layout/include/applet_metadata.h
cp $(execpath :include/autoconf.h)                      $(RULEDIR)/host_layout/include/autoconf.h
cp $(execpath :include/applets.h)                       $(RULEDIR)/host_layout/include/applets.h
cp $(execpath :include/usage.h)                         $(RULEDIR)/host_layout/include/usage.h
""",
    )

    cc_library(
        name = "host_layout_headers",
        hdrs = [
            "host_layout/include/applet_metadata.h",
            "host_layout/include/autoconf.h",
            "host_layout/include/applets.h",
            "host_layout/include/usage.h",
        ],
        includes = ["host_layout/include"],  # for usage.c's bare `#include "applets.h"`
    )

    # applet_tables: scans applets.h for APPLET_* records and emits the table.
    cc_binary(
        name = "applet_tables_host",
        srcs = ["host_layout/applets/applet_tables.c"],
        copts = ["-w"],
        deps = [":host_layout_headers"],
        visibility = ["//visibility:private"],
    )

    # usage: emits the packed help strings. usage_compressed shell wraps it.
    cc_binary(
        name = "usage_host",
        srcs = ["host_layout/applets/usage.c"],
        copts = ["-w"],
        deps = [":host_layout_headers"],
        visibility = ["//visibility:private"],
    )

    # ─── Codegen genrules ───────────────────────────────────────────────────

    # mkconfigs reads .config in cwd, writes argv[1] + argv[2].
    _stage_tree_and_run(
        name = "bbconfigopts_h",
        srcs = [],
        tools = [],
        outs = ["include/bbconfigopts.h", "include/bbconfigopts_bz2.h"],
        cmd_body = "sh scripts/mkconfigs include/bbconfigopts.h include/bbconfigopts_bz2.h",
    )

    # generate_BUFSIZ.sh: reads .config, writes argv[1].
    _stage_tree_and_run(
        name = "common_bufsiz_h",
        srcs = [],
        tools = [],
        outs = ["include/common_bufsiz.h"],
        cmd_body = "sh scripts/generate_BUFSIZ.sh include/common_bufsiz.h",
    )

    # embedded_scripts: reads .config + embed/ + applets_sh/, writes argv[1].
    _stage_tree_and_run(
        name = "embedded_scripts_h",
        srcs = [":include/applets.h", ":include/autoconf.h"],
        tools = [],
        outs = ["include/embedded_scripts.h"],
        # `srctree` and `HOSTCC` are normally exported by the kbuild Makefile;
        # the script reads them implicitly. Without them busybox.mkscripts
        # silently returns no scripts and the header gets NUM_SCRIPTS=0
        # regardless of CONFIG_FEATURE_SH_EMBEDDED_SCRIPTS. busybox.mkscripts
        # also runs `$HOSTCC -E -include include/autoconf.h include/applets.h`,
        # so both must be present in cwd/include/.
        cmd_body = """\
mkdir -p include
cp "$$EXEC_ROOT/$(execpath :include/applets.h)"  include/applets.h
cp "$$EXEC_ROOT/$(execpath :include/autoconf.h)" include/autoconf.h
srctree=. HOSTCC=cc sh scripts/embedded_scripts include/embedded_scripts.h embed applets_sh
""",
    )

    # gen_build_files.sh: scans ALL .c files for //applet:, //usage:,
    # //kbuild:, //config: markers, emits applets.h + usage.h, and
    # materializes */Kbuild + */Config.in from their `.src` siblings.
    _stage_tree_and_run(
        name = "gen_build_files",
        srcs = [],
        tools = [],
        outs = ["include/applets.h", "include/usage.h"],
        cmd_body = "sh scripts/gen_build_files.sh . .",
    )

    # kconfig conf in silent-oldconfig mode: reads Config.in (which sources
    # subdir Config.ins, all materialized by :gen_build_files), reads .config,
    # writes include/autoconf.h.
    _stage_tree_and_run(
        name = "autoconf_h",
        srcs = [],
        tools = [":kconfig_conf_host"],
        outs = ["include/autoconf.h"],
        cmd_body = """\
mkdir -p include
# Materialize */Kbuild + */Config.in from their `.src` siblings so the
# Config.in tree conf walks resolves cleanly.
sh scripts/gen_build_files.sh . .
"$$EXEC_ROOT/$(execpath :kconfig_conf_host)" -s Config.in
""",
    )

    # applet_tables: emits the applet table. Needs autoconf.h + applets.h
    # already in `include/` to compile, but since it's a *runtime* invocation
    # of the host binary, we only need the binary itself + the include/ files
    # we already have via this staged tree.
    _stage_tree_and_run(
        name = "applet_tables_h",
        srcs = [],
        tools = [":applet_tables_host"],
        outs = ["include/applet_tables.h", "include/NUM_APPLETS.h"],
        cmd_body = """\
"$$EXEC_ROOT/$(execpath :applet_tables_host)" include/applet_tables.h include/NUM_APPLETS.h
""",
    )

    # usage_compressed.h: pipe usage_host's output through the shipped
    # usage_compressed shell wrapper.
    _stage_tree_and_run(
        name = "usage_compressed_h",
        srcs = [],
        tools = [":usage_host"],
        outs = ["include/usage_compressed.h"],
        cmd_body = """\
mkdir -p _usage_stage
cp "$$EXEC_ROOT/$(execpath :usage_host)" _usage_stage/usage
chmod +x _usage_stage/usage
sh applets/usage_compressed include/usage_compressed.h _usage_stage
""",
    )
