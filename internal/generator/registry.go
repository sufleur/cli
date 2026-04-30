package generator

import "fmt"

var registry = map[string]func() Generator{}

// Register adds a generator constructor for the given language.
func Register(language string, constructor func() Generator) {
	registry[language] = constructor
}

// Get returns a new Generator for the given language.
func Get(language string) (Generator, error) {
	constructor, ok := registry[language]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %q", language)
	}
	return constructor(), nil
}
