# Embedded NetProxy core

This directory contains the runtime subset of NetProxy v8.6.1 used by
Glovebox: `netproxyctl`, sing-box, default configuration, and rule data. The
upstream NetProxy terminal is vendored under `web/netproxy` and connected to
the embedded runtime through Glovebox's authenticated, netproxyctl-only HTTP
bridge. The NetProxy APK and separate sing-box dashboard are omitted because
Glovebox remains the single management interface for this package.

NetProxy is licensed under GNU GPL v3. The complete corresponding source for
this binary release starts from upstream commit
`3fd2a1cb9710b59f2723d3c0bb2a01c7090e0998` and is available from:

https://github.com/Fanju6/NetProxy-Magisk/tree/v8.6.1

Glovebox applies the source patch in `patches/` before building
`bin/netproxyctl`. The unmodified upstream license and notice are included as
`LICENSE` and `NOTICE`.
