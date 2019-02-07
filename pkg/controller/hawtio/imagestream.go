package hawtio

import (
	imagev1 "github.com/openshift/api/image/v1"
)

func imageStreamContainsTag(is *imagev1.ImageStream, tag string) (bool, *imagev1.TagReference) {
	for _, t := range is.Spec.Tags {
		if t.Name == tag {
			return true, &t
		}
	}
	return false, nil
}
