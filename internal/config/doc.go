// Package config resolves effective configuration across layers:
// defaults -> user file -> project file (allowlisted keys only) -> WT_* env ->
// flags.
//
// The project layer is untrusted: it comes from a cloned repository, so only
// allowlisted keys are honoured and declared hooks require explicit approval.
//
// Decision/effect boundary: resolution is a pure function of the layers; reading
// the files is the caller's job.
package config
