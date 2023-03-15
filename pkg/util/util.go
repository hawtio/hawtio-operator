package util

import (
	"io/ioutil"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

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
