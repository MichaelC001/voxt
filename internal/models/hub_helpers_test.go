package models

import fammodels "github.com/hehehai/actnowkit/models"

// Hub is the family's default model library
// (~/Library/Application Support/ActNow/models/hub, spec v1.0.9): the
// real one, for the acceptance tests run by hand.
func Hub() string { return fammodels.DefaultHub() }
