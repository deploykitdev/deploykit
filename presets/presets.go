// Package presets ships the curated catalog of service presets (database
// images and similar) that the canvas uses to pre-fill new service drafts.
//
// The catalog is authored as one YAML file per preset under data/, embedded
// into the binary via go:embed and parsed once at startup.
package presets

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/deploykitdev/deploykit"
	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var presetsFS embed.FS

// yamlPreset mirrors deploykit.Preset for YAML decoding. We keep it private
// so deploykit's domain types don't need to know about YAML field names.
type yamlPreset struct {
	ID      string           `yaml:"id"`
	Name    string           `yaml:"name"`
	Image   string           `yaml:"image"`
	IconURL string           `yaml:"icon_url"`
	Ports   []yamlPort       `yaml:"ports,omitempty"`
	EnvVars []yamlPresetEnv  `yaml:"env_vars"`
}

type yamlPort struct {
	ContainerPort int    `yaml:"container_port"`
	HostPort      int    `yaml:"host_port,omitempty"`
	Protocol      string `yaml:"protocol,omitempty"`
}

type yamlPresetEnv struct {
	Key      string `yaml:"key"`
	Value    string `yaml:"value,omitempty"`
	Generate string `yaml:"generate,omitempty"`
}

// Service implements deploykit.PresetService over the embedded YAML catalog.
type Service struct {
	byID map[string]*deploykit.Preset
	all  []*deploykit.Preset
}

// New loads and validates every preset under data/. Returns a non-nil error
// if any file fails to parse, references an unknown generator, or has a
// duplicate id.
func New() (*Service, error) {
	entries, err := fs.ReadDir(presetsFS, "data")
	if err != nil {
		return nil, fmt.Errorf("reading embedded preset directory: %w", err)
	}

	byID := make(map[string]*deploykit.Preset, len(entries))
	all := make([]*deploykit.Preset, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := "data/" + entry.Name()
		raw, err := presetsFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading preset %s: %w", path, err)
		}

		var yp yamlPreset
		if err := yaml.Unmarshal(raw, &yp); err != nil {
			return nil, fmt.Errorf("parsing preset %s: %w", path, err)
		}
		p := convertPreset(yp)
		if err := validatePreset(&p, path); err != nil {
			return nil, err
		}
		if _, dup := byID[p.ID]; dup {
			return nil, fmt.Errorf("duplicate preset id %q (in %s)", p.ID, path)
		}

		byID[p.ID] = &p
		all = append(all, &p)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	return &Service{byID: byID, all: all}, nil
}

// List returns the preset catalog (specs only, no generators run).
func (s *Service) List(_ context.Context) ([]*deploykit.Preset, error) {
	out := make([]*deploykit.Preset, len(s.all))
	for i, p := range s.all {
		out[i] = clonePreset(p)
	}
	return out, nil
}

// Get returns a deep-copy of the preset with every Generate-typed env var
// replaced by a freshly generated literal Value.
func (s *Service) Get(_ context.Context, id string) (*deploykit.Preset, error) {
	p, ok := s.byID[id]
	if !ok {
		return nil, deploykit.Errorf(deploykit.ENOTFOUND, "Preset %q not found.", id)
	}
	out := clonePreset(p)
	for i := range out.EnvVars {
		ev := &out.EnvVars[i]
		if ev.Generate == "" {
			continue
		}
		v, err := generate(ev.Generate)
		if err != nil {
			return nil, fmt.Errorf("generating value for %s.%s: %w", out.ID, ev.Key, err)
		}
		ev.Value = v
		ev.Generate = ""
	}
	return out, nil
}

func validatePreset(p *deploykit.Preset, path string) error {
	if p.ID == "" {
		return fmt.Errorf("preset %s: id is required", path)
	}
	if p.Name == "" {
		return fmt.Errorf("preset %s: name is required", path)
	}
	if p.Image == "" {
		return fmt.Errorf("preset %s: image is required", path)
	}
	for i, ev := range p.EnvVars {
		if ev.Key == "" {
			return fmt.Errorf("preset %s: env_vars[%d].key is required", path, i)
		}
		if (ev.Value == "") == (ev.Generate == "") {
			return fmt.Errorf("preset %s: env_vars[%d] (%s) must set exactly one of value or generate", path, i, ev.Key)
		}
		if ev.Generate != "" && !hasGenerator(ev.Generate) {
			return fmt.Errorf("preset %s: env_vars[%d] (%s) references unknown generator %q", path, i, ev.Key, ev.Generate)
		}
	}
	return nil
}

func convertPreset(yp yamlPreset) deploykit.Preset {
	p := deploykit.Preset{
		ID:      yp.ID,
		Name:    yp.Name,
		Image:   yp.Image,
		IconURL: yp.IconURL,
	}
	if len(yp.Ports) > 0 {
		p.Ports = make([]deploykit.PortMapping, len(yp.Ports))
		for i, yport := range yp.Ports {
			p.Ports[i] = deploykit.PortMapping{
				ContainerPort: yport.ContainerPort,
				HostPort:      yport.HostPort,
				Protocol:      yport.Protocol,
			}
		}
	}
	if len(yp.EnvVars) > 0 {
		p.EnvVars = make([]deploykit.PresetEnvVar, len(yp.EnvVars))
		for i, yev := range yp.EnvVars {
			p.EnvVars[i] = deploykit.PresetEnvVar{
				Key:      yev.Key,
				Value:    yev.Value,
				Generate: yev.Generate,
			}
		}
	}
	return p
}

func clonePreset(p *deploykit.Preset) *deploykit.Preset {
	out := *p
	if p.Ports != nil {
		out.Ports = append([]deploykit.PortMapping(nil), p.Ports...)
	}
	if p.EnvVars != nil {
		out.EnvVars = append([]deploykit.PresetEnvVar(nil), p.EnvVars...)
	}
	return &out
}
