package webhook

import (
	"context"
	"encoding/json"
	"net/http"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type HawtioGuard struct {
	decoder admission.Decoder
}

// InjectDecoder injects the decoder automatically from controller-runtime
func (m *HawtioGuard) InjectDecoder(d admission.Decoder) error {
	m.decoder = d
	return nil
}

// Handle monitors Hawtio Custom Resources and ensures that the
// modified-by annotation has the value of the editor of the resource
func (m *HawtioGuard) Handle(ctx context.Context, req admission.Request) admission.Response {
	hawtio := &hawtiov2.Hawtio{}

	// Decode incoming Hawtio CR
	err := m.decoder.Decode(req, hawtio)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Extract authenticated user identity
	actingUser := req.UserInfo.Username

	// Initialize annotations map if it doesn't exist
	if hawtio.Annotations == nil {
		hawtio.Annotations = make(map[string]string)
	}

	// Overwrite or set the creator annotation
	hawtio.Annotations["hawtio.io/last-modified-by"] = actingUser

	// Marshal the modified object back to JSON
	marshaledHawtio, err := json.Marshal(hawtio)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Return a JSONPatch response back to the API server
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledHawtio)
}
