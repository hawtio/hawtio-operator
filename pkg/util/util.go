package util

import (
	"io/ioutil"
	"regexp"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/util/yaml"
)

var log = logf.Log.WithName("util")

func jsonIfYaml(source []byte, filename string) ([]byte, error) {
	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		return yaml.ToJSON(source)
	}

	return source, nil
}

func LoadConfigFromFile(path string) ([]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	data, err = jsonIfYaml(data, path)
	if err != nil {
		return nil, err
	}

	return data, err
}

func Contains(strs []string, item string) bool {
	for _, s := range strs {
		if s == item {
			return true
		}
	}

	return false
}

// MatchPatterns returns true if target matches any of the given patterns.
func MatchPatterns(patterns []string, target string) bool {
	match := false
	for _, p := range patterns {
		if Match(p, target) {
			match = true
			break
		}
	}

	return match
}

// Match provides simple string pattern match that only supports wildcard '*'.
func Match(pattern, str string) bool {
	var b strings.Builder
	_, err := b.WriteRune('^')
	if err != nil {
		return false
	}
	for _, c := range pattern {
		var s string
		switch c {
		case '*':
			s = ".*"
		case '.', '+', '?', '{', '}', '(', ')', '[', ']', '|', '\\', '^', '$':
			s = "\\" + string(c)
		default:
			s = string(c)
		}
		_, err := b.WriteString(s)
		if err != nil {
			return false
		}
	}
	_, err = b.WriteRune('$')
	if err != nil {
		return false
	}

	match, err := regexp.MatchString(b.String(), str)
	if err != nil {
		return false
	}
	return match
}
