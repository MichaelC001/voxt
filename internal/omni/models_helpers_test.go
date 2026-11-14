package omni

// Test-only lists of the catalogue's models.

// Models are the curated models: the speech models (recommended first),
// then Silero VAD. Copies.
func Models() []Spec {
	out := make([]Spec, len(specs))
	for i, s := range specs {
		out[i] = s.clone()
	}
	return out
}

// ASRModels are the curated speech models, the recommended one first.
func ASRModels() []Spec {
	var out []Spec
	for _, s := range specs {
		if s.ASR != nil {
			out = append(out, s.clone())
		}
	}
	return out
}
