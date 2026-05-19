"""Starlark parser for the regular subset of HCL that terraform emits to
`.terraform.lock.hcl`.

Terraform's own writer produces a strict, line-oriented format:

    # comment
    # comment

    provider "registry.terraform.io/hashicorp/google" {
      version     = "7.32.0"
      constraints = "~> 7.0"
      hashes = [
        "h1:...",
        "zh:...",
      ]
    }

This module parses that grammar — and only that grammar — into:

    {
      "hashicorp/google": {
        "version": "7.32.0",
        "constraints": "~> 7.0",
        "hashes": ["h1:...", "zh:..."],
      },
      ...
    }

The `registry.terraform.io/` prefix is stripped from the provider address
since the bazel side only cares about the `<namespace>/<type>` portion.

Limitations (we don't parse them because terraform doesn't write them):
- Single-line arrays (`hashes = ["a", "b"]`).
- Nested blocks inside `provider`.
- HCL2 heredocs / templated strings.
"""

def parse_terraform_lockfile(content):
    """Parse `.terraform.lock.hcl` text. Returns a dict keyed by source
    address (registry prefix stripped).
    """
    out = {}
    state = "TOP"
    current_addr = None
    current_block = None
    current_array_field = None

    for raw in content.split("\n"):
        line = raw.strip()
        if line == "" or line.startswith("#"):
            continue

        if state == "TOP":
            # Expect: provider "<address>" {
            if line.startswith("provider "):
                # The address is the contents of the FIRST quoted string
                # on the line. The block-open `{` follows the closing quote
                # (possibly with whitespace).
                first_q = line.find('"')
                second_q = line.find('"', first_q + 1)
                if first_q == -1 or second_q == -1:
                    fail("malformed provider header (missing quoted address): " + raw)
                addr = line[first_q + 1:second_q]
                if addr.startswith("registry.terraform.io/"):
                    addr = addr[len("registry.terraform.io/"):]
                current_addr = addr
                current_block = {}
                state = "BLOCK"

            # Any other top-level line is ignored (no other constructs
            # appear in a `terraform providers lock` output).

        elif state == "BLOCK":
            if line == "}":
                out[current_addr] = current_block
                current_addr = None
                current_block = None
                state = "TOP"
            elif line.endswith("= ["):
                # Multi-line array begins. Key is everything before the
                # first `=`; the array body follows on subsequent lines.
                key = line[:line.find("=")].strip()
                current_array_field = key
                current_block[key] = []
                state = "ARRAY"
            elif "=" in line:
                eq = line.find("=")
                key = line[:eq].strip()
                val = line[eq + 1:].strip()
                if val.startswith('"') and val.endswith('"'):
                    val = val[1:-1]
                current_block[key] = val

        elif state == "ARRAY":
            if line.startswith("]"):
                state = "BLOCK"
                current_array_field = None
            else:
                # `"h1:abc...",`  → strip optional trailing comma + quotes.
                val = line
                if val.endswith(","):
                    val = val[:-1]
                val = val.strip()
                if val.startswith('"') and val.endswith('"'):
                    val = val[1:-1]
                current_block[current_array_field].append(val)

    if state != "TOP":
        fail("unterminated block while parsing lockfile (state={})".format(state))
    return out
