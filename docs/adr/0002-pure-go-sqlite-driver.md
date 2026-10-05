# Embed SQLite through the pure-Go driver, not the cgo one

Seraph embeds SQLite through a pure-Go driver, accepting a larger executable and a larger
dependency tree in exchange for two properties that are expensive to recover later.

First, a single build produces a working executable for every target platform. A cgo
dependency cannot be cross-compiled without also provisioning a C toolchain for each
destination, so a cgo build can only be produced for the machine it was built on.

Second, full-text search is present in *every* build. Under the cgo driver it is gated
behind a compile flag, which means the author's executable has it, an install performed by
someone else does not, and the failure appears at runtime on a user's machine rather than at
build time. That is a failure mode worth any amount of binary size.

**Considered**: the cgo driver, which builds faster and yields a smaller executable.
Rejected for the two reasons above. The dependency's source living outside the primary
forge was weighed and accepted as a known cost.
