package obfusps

import (
	"github.com/benzoXdev/obfusps/internal/engine"
)

type Config = engine.Options

func Obfuscate(source string, cfg Config) (string, error) {
	return engine.ObfuscateString(source, cfg)
}
