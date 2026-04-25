package deploykit

import "context"

// Preset describes a service template (e.g. PostgreSQL) that pre-fills the
// service draft form on the canvas with the right Docker image, icon, and
// the env vars the image needs to run.
type Preset struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Image   string         `json:"image"`
	IconURL string         `json:"icon_url"`
	Ports   []PortMapping  `json:"ports,omitempty"`
	EnvVars []PresetEnvVar `json:"env_vars"`
}

// PresetEnvVar is one env var on a Preset. Exactly one of Value or Generate
// must be set: Value is a literal default, Generate names a random-value
// generator that runs server-side at materialization time.
type PresetEnvVar struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	Generate string `json:"generate,omitempty"`
}

// PresetService exposes the curated set of service presets. Implementations
// are expected to load the catalog once at construction time.
type PresetService interface {
	// List returns the preset specs (generators not run). Suitable for
	// rendering a picker — the dialog only needs name/image/icon.
	List(ctx context.Context) ([]*Preset, error)

	// Get returns a copy of the preset with every Generate-typed env var
	// replaced by a freshly generated literal Value. Each call produces
	// fresh random values.
	// Returns ENOTFOUND if id does not match any preset.
	Get(ctx context.Context, id string) (*Preset, error)
}
