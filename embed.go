// Package bestcandidate carries files embedded into the binary.
package bestcandidate

import _ "embed"

// ExampleProfile is the template written by `best-candidate init`.
//
//go:embed examples/profile.example.md
var ExampleProfile string
