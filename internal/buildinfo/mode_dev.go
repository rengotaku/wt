//go:build dev

package buildinfo

// IsDev is true for `-tags dev` builds (make run / air hot-reload). The web
// UI hides the stale-binary badge in this mode because air rebuilds and
// restarts on file changes.
const IsDev = true
