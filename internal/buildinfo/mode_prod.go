//go:build !dev

package buildinfo

// IsDev is false for production builds (make build). See mode_dev.go for the
// dev-tag counterpart.
const IsDev = false
