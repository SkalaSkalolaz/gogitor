package language

import (
	"path/filepath"
	"sort"
	"strings"
)

// ID identifies a programming language supported by Gogitor.
type ID string

const (
	Go ID = "go"
)

// Definition describes the language-specific identity known to the application.
//
// The registry intentionally keeps this layer small. Language-specific execution
// remains behind the existing runner/agent services so adding a language does not
// require changing command routing, TUI code, Git, or patch handling.
type Definition struct {
	ID         ID
	Name       string
	Extensions []string
}

// Registry is the single source of truth for language detection in the application.
// New languages should be added by registering another Definition here and later
// connecting their toolchain to the execution layer.
type Registry struct {
	byID  map[ID]Definition
	byExt map[string]ID
}

func NewRegistry() *Registry {
	r := &Registry{
		byID:  make(map[ID]Definition),
		byExt: make(map[string]ID),
	}
	r.Register(Definition{
		ID:   Go,
		Name: "Go",
		Extensions: []string{
			".go",
		},
	})
	return r
}

func (r *Registry) Register(def Definition) {
	if r == nil || def.ID == "" {
		return
	}
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		def.Name = string(def.ID)
	}

	normalized := make([]string, 0, len(def.Extensions))
	for _, ext := range def.Extensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		normalized = append(normalized, ext)
		r.byExt[ext] = def.ID
	}
	def.Extensions = normalized
	r.byID[def.ID] = def
}

func (r *Registry) Detect(path string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	id, ok := r.byExt[ext]
	if !ok {
		return Definition{}, false
	}
	def, ok := r.byID[id]
	return def, ok
}

func (r *Registry) HasPath(path string) bool {
	_, ok := r.Detect(path)
	return ok
}

func (r *Registry) Get(id ID) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	def, ok := r.byID[id]
	return def, ok
}

func (r *Registry) IDs() []ID {
	if r == nil {
		return nil
	}
	ids := make([]ID, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
