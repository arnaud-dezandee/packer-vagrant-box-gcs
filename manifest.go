package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Provider struct {
	Name         string `json:"name"`
	Url          string `json:"url"`
	ChecksumType string `json:"checksum_type"`
	Checksum     string `json:"checksum"`
}

type Version struct {
	Version   string      `json:"version"`
	Providers []*Provider `json:"providers"`
}

type Manifest struct {
	Name     string     `json:"name"`
	Versions []*Version `json:"versions"`
}

func (m *Manifest) add(version string, provider *Provider) error {
	for _, w := range m.Versions {
		if w.Version == version {
			for _, p := range w.Providers {
				if p.Name == provider.Name {
					return fmt.Errorf("%s box already exists in manifest for version %s", p.Name, version)
				}
			}
			w.Providers = append(w.Providers, provider)
			return nil
		}
	}
	m.Versions = append(m.Versions, &Version{
		Version:   version,
		Providers: []*Provider{provider},
	})
	return nil
}

func (m *Manifest) NewReader() (io.Reader, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}

	return strings.NewReader(buf.String()), nil
}
