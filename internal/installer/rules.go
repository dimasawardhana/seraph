package installer

import _ "embed"

//go:embed rules.md
var Rules string

const StateIgnore = `# Seraph local state — machine-local, not for committing.
#
# The rendered snapshots alongside this file are meant to be committed.
state.db
state.db-wal
state.db-shm
`

const (
	StartMarker = "<!-- SERAPH RULES START -->"
	EndMarker   = "<!-- SERAPH RULES END -->"
)
