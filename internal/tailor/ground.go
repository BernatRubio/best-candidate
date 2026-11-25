package tailor

import "github.com/BernatRubio/best-candidate/internal/profile"

// normalize is the profile's token normalisation, so that text in the
// documents is compared with the profile in the same way.
func normalize(s string) string { return profile.Normalize(s) }
