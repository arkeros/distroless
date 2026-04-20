"""Bifrost version definitions with filenames and checksums.

Binaries are published to GitHub Releases at:

    https://github.com/arkeros/senku/releases/download/bifrost/v{}/{}

Format: "VERSION-PLATFORM": (filename, sha256)
Platforms: darwin_amd64, darwin_arm64, linux_amd64, linux_arm64

Regenerate with: bazel run //bazel/cmd/knife -- prebuilts update --tool bifrost
"""

DEFAULT_VERSION = "2026.17.75"

BIFROST_VERSIONS = {
    "2026.15.64-darwin_amd64": (
        "bifrost-darwin-amd64",
        "45a1461bb90e420dbd88f125493d3b932679cec706d2f67041dcad64873aa77f",
    ),
    "2026.15.64-darwin_arm64": (
        "bifrost-darwin-arm64",
        "9ddb879fdb355c95dcb057db25cb34210bf0ebcb98dc949ddc7247a4feb73fce",
    ),
    "2026.15.64-linux_amd64": (
        "bifrost-linux-amd64",
        "1c36d6d685d4c3731036cebdd7e7e090f40ebbdc3463f56b1aba9d2d71fbf3e6",
    ),
    "2026.15.64-linux_arm64": (
        "bifrost-linux-arm64",
        "1bf4952363af3295b6e9eed2eec66ba87cc8f9f0b9fef716247e1676f3f750ca",
    ),
    "2026.15.71-darwin_amd64": (
        "bifrost-darwin-amd64",
        "3005e367d27a0fe12f438fc549ff79ca0317fb6fbc5a148c53e592a3c592df26",
    ),
    "2026.15.71-darwin_arm64": (
        "bifrost-darwin-arm64",
        "cee142ef92512814123f25d2f7f4a5e7c143af8b1040760694ff85dd1d879064",
    ),
    "2026.15.71-linux_amd64": (
        "bifrost-linux-amd64",
        "f4239107d5a5772e66d5bbc623ae225f8974649a939ceec8b4695f2f467785cc",
    ),
    "2026.15.71-linux_arm64": (
        "bifrost-linux-arm64",
        "27c67d77cdccb609a2dccd68454b8ba0df576ed200e02ba2183e3a11b624cb8f",
    ),
    "2026.15.73-darwin_amd64": (
        "bifrost-darwin-amd64",
        "3c92a6b1eb89bfc486ffc1bd67bea355afbb5c0929c1f9f631b57cdae99e5d54",
    ),
    "2026.15.73-darwin_arm64": (
        "bifrost-darwin-arm64",
        "186bc8fc16ba22e91091579ecad15dd2227d8cca2475e7747751b9055f826ca8",
    ),
    "2026.15.73-linux_amd64": (
        "bifrost-linux-amd64",
        "3f4e45f6bed2e93525a5a78b5b2f46713cd7881797729d3991246b0ed4853e47",
    ),
    "2026.15.73-linux_arm64": (
        "bifrost-linux-arm64",
        "4f0ef09c93c14b81ed20489e2fa8737c7225ec4e21dffcd880cfcdab2d301485",
    ),
    "2026.15.76-darwin_amd64": (
        "bifrost-darwin-amd64",
        "19a978f6dbc6b7c3169148891d2091ac499df3fb3ab0585e3651be0797c6ab73",
    ),
    "2026.15.76-darwin_arm64": (
        "bifrost-darwin-arm64",
        "7791566bf72e9f1121f9bf5cf3beafcfafc58904a0fec2bf78fe893896a53e5b",
    ),
    "2026.15.76-linux_amd64": (
        "bifrost-linux-amd64",
        "d08f866b0e78dcb565c3ab2c6e0d62b069429f2526b7c322d04538a8c707858a",
    ),
    "2026.15.76-linux_arm64": (
        "bifrost-linux-arm64",
        "1d28a0002d208550a1c3ee0a72fd941010304d4a5625a3f14bf4f03042747c43",
    ),
    "2026.16.6-darwin_amd64": (
        "bifrost-darwin-amd64",
        "df6a8659b53585594b12c333a0009958fa9567c2733ae44779dfa3317c665601",
    ),
    "2026.16.6-darwin_arm64": (
        "bifrost-darwin-arm64",
        "09942a238f82974ce879c8886e41f64877d76ee408c2f9d3a20dfd05c3c7164d",
    ),
    "2026.16.6-linux_amd64": (
        "bifrost-linux-amd64",
        "0a591eb3f4db260d9b6572b54cd361895fff3207cbc2774cedb58cfee4435234",
    ),
    "2026.16.6-linux_arm64": (
        "bifrost-linux-arm64",
        "421156df1b061ed536fa8d5594897ae59f853ccaa63a3fba97656def3228f934",
    ),
    "2026.16.18-darwin_amd64": (
        "bifrost-darwin-amd64",
        "ca2f0b25efdc231b6d83a8c5ded3a52fbc1f4b344c0c9d03b262317eb751c28d",
    ),
    "2026.16.18-darwin_arm64": (
        "bifrost-darwin-arm64",
        "c1fcd2f3fbe2dcf6d1eb6fe197e43b63f8ac3db0dc61d266c1b9468a4e1c8631",
    ),
    "2026.16.18-linux_amd64": (
        "bifrost-linux-amd64",
        "db3b7ad0611749a3250279ad0d2747c3afcc876c1b2d4c1edbb9065628599a6b",
    ),
    "2026.16.18-linux_arm64": (
        "bifrost-linux-arm64",
        "592af6f983d9d7eb811023a11c59907bf38bfa3984f040a5175493b8536682d0",
    ),
    "2026.16.21-darwin_amd64": (
        "bifrost-darwin-amd64",
        "c72881e4b3c689c32f7e7f9f708fbe26490bc060cdb40070921ce76628ac7c56",
    ),
    "2026.16.21-darwin_arm64": (
        "bifrost-darwin-arm64",
        "c26a96c65f9eafcb4b51eb4e2dbe76ed2bf0ee032ab85d8f6c866b48d1e0a47c",
    ),
    "2026.16.21-linux_amd64": (
        "bifrost-linux-amd64",
        "aebbed24443c402774b4fdd79568cbff961df4667c38ae0d4f2aa9cebf4c977c",
    ),
    "2026.16.21-linux_arm64": (
        "bifrost-linux-arm64",
        "e74878c56b6abaad3641733199eec22da171af3e44bd8f5c16e1eec77a26c687",
    ),
    "2026.16.24-darwin_amd64": (
        "bifrost-darwin-amd64",
        "f3ba6cc634ae8b2e68df5b672cf043196dcd3e4b6809af68420a13db1e06cf8f",
    ),
    "2026.16.24-darwin_arm64": (
        "bifrost-darwin-arm64",
        "118488602a2705def4ea1437813d590b86276576dbf1cf93f2d086d55318dd40",
    ),
    "2026.16.24-linux_amd64": (
        "bifrost-linux-amd64",
        "be56892d68cffd6e505b493fe483add1138a7bfe942a2003ce7cc75ca568b7f7",
    ),
    "2026.16.24-linux_arm64": (
        "bifrost-linux-arm64",
        "3f68e1ec4ab4518e067562f4d40cceced64cef6758185f5dd26089f7ae49541c",
    ),
    "2026.16.43-darwin_amd64": (
        "bifrost-darwin-amd64",
        "d747383e3d8ed96e8ede2463922e6d4eed089772a98e3ab9dc347282716cde84",
    ),
    "2026.16.43-darwin_arm64": (
        "bifrost-darwin-arm64",
        "6418d84b5f07635210b7f050a8e3b5e8660376943d1045762f4ea3a467afac32",
    ),
    "2026.16.43-linux_amd64": (
        "bifrost-linux-amd64",
        "9c0722ad39e4316b4d7c33eb89e5f656de7f4da152482cdfb22139ba4bcf94a1",
    ),
    "2026.16.43-linux_arm64": (
        "bifrost-linux-arm64",
        "28d9aebc3c197d64825690d98b3203ada290ca71fecce09ce296faee42855b5c",
    ),
    "2026.17.75-darwin_amd64": (
        "bifrost-darwin-amd64",
        "949bcb31b75a7502f94e1c8f557e167d9b88b4f2981097813d28e3d1e37a28dc",
    ),
    "2026.17.75-darwin_arm64": (
        "bifrost-darwin-arm64",
        "69fc0604389444ee99ee4511cbff8d65dc9ad17f3f76ff330f88973ffe2bc9a6",
    ),
    "2026.17.75-linux_amd64": (
        "bifrost-linux-amd64",
        "9f4e7901365d8aae301394683dd20b60bcbf2a4be627821c0becab7470e54022",
    ),
    "2026.17.75-linux_arm64": (
        "bifrost-linux-arm64",
        "00943bec2a97db86f7ac84b02bd12c2eb6ec3fb1908bfdeb7afe6c7345b0dc60",
    ),
}

def get_bifrost_url(version, filename):
    """Returns the download URL for a bifrost release."""
    return "https://github.com/arkeros/senku/releases/download/bifrost/v{}/{}".format(version, filename)
