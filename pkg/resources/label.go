package resources

const (
	labelAppKey      = "app"
	labelResourceKey = "deployment"
)

// Set labels in a map
func labelsForHawtio(name string) map[string]string {
	return map[string]string{
		labelAppKey:      "hawtio",
		labelResourceKey: name,
	}
}
