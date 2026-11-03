package main

import (
	"fmt"
	"os"
	"strings"
)

func substitute(input string, cfg config) (string, error) {
	var output strings.Builder
	for {
		start := strings.Index(input, "${")
		if start < 0 {
			output.WriteString(input)
			return output.String(), nil
		}
		output.WriteString(input[:start])
		input = input[start:]
		end := strings.IndexAny(input, "}\r\n")
		if end < 0 || input[end] != '}' {
			return "", fmt.Errorf("unclosed environment placeholder")
		}
		placeholder := input[:end+1]
		key, fallback, hasFallback := strings.Cut(input[2:end], ":-")
		if key == "" || (len(cfg.whitelist) > 0 && !cfg.whitelist[key]) || cfg.blacklist[key] {
			output.WriteString(placeholder)
		} else if value, exists := os.LookupEnv(key); exists {
			output.WriteString(value)
		} else if hasFallback {
			output.WriteString(fallback)
		} else if cfg.failOnMissing {
			// Neither template contents nor environment values belong in logs.
			return "", fmt.Errorf("a required environment variable is missing")
		} else {
			output.WriteString(placeholder)
		}
		input = input[end+1:]
	}
}
