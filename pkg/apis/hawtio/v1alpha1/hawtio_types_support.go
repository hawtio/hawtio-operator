package v1alpha1

func (hawtio *Hawtio) IsRbacEnabled() bool {
	return hawtio.Spec.RBAC.Enabled == nil || *hawtio.Spec.RBAC.Enabled
}
