package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var log = logf.Log.WithName("util")
var logLevelEnvVar = "OPERATOR_LOG_LEVEL"

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

// MergeMap copies all key/value pairs from 'required' into 'existing'.
// It adds new keys and updates existing ones, but DOES NOT remove keys
// that are in 'existing' but not in 'required'.
func MergeMap(existing, required map[string]string) map[string]string {
	if existing == nil {
		return required
	}
	for k, v := range required {
		existing[k] = v
	}
	return existing
}

// ReportDiff logs the difference between two objects, ignoring system-managed fields.
// Only execute comparisons if the log level is "debug"
func ReportDiff(resourceName string, live client.Object, desired client.Object) {
	lvl := os.Getenv(logLevelEnvVar)
	if !strings.EqualFold(lvl, "debug") {
		return
	}

	// Ignore standard system fields that always change or don't matter for logic
	ignoredMeta := cmpopts.IgnoreFields(metav1.ObjectMeta{},
		"ResourceVersion", "UID",
		"CreationTimestamp", "Generation",
		"ManagedFields", "OwnerReferences",
		"SelfLink")

	// We compare the Live state (live) against the Desired state (desired)
	diff := cmp.Diff(live, desired, ignoredMeta)
	if diff != "" {
		log.V(DebugLogLevel).Info(fmt.Sprintf("⚠️  DIFF DETECTED: %s", resourceName), "diff", diff)
	}
}

// ReportResourceChange checks if an update occurred and logs the
// specific details used for tracing ripple effects.
func ReportResourceChange(kind string, obj client.Object, result controllerutil.OperationResult) {
	lvl := os.Getenv(logLevelEnvVar)
	if !strings.EqualFold(lvl, "debug") {
		return
	}

	if result != controllerutil.OperationResultNone {
		log.V(DebugLogLevel).Info("🚨 RESOURCE CHANGE DETECTED",
			"Kind", kind,
			"Name", obj.GetName(),
			"Operation", result,
			"NewResourceVersion", obj.GetResourceVersion(),
		)
	}
}
