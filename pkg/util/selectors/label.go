package selectors

const (
	LabelAppKey      = "app"
	LabelResourceKey = "deployment"
)

// Set labels in a map
func LabelsForHawtio(name string) map[string]string {
	return map[string]string{
		LabelAppKey:      "hawtio",
		LabelResourceKey: name,
	}
}
