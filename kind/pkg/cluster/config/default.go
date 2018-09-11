/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

// New returns a new default Config
func New() *Config {
	cfg := &Config{}
	cfg.ApplyDefaults()
	return cfg
}

// ApplyDefaults replaces unset fields with defaults
func (c *Config) ApplyDefaults() {
	if c.Image == "" {
		c.Image = "kind-node"
	}
	if c.NumNodes == 0 {
		c.NumNodes = 1
	}
}
