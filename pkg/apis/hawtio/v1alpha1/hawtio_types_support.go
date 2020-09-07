package v1alpha1

func (hawtio *Hawtio) IsRbacEnabled(isOpenShift4 bool) bool {
	return hawtio.Spec.RBAC.Enabled == nil && isOpenShift4 ||
		hawtio.Spec.RBAC.Enabled != nil && *hawtio.Spec.RBAC.Enabled
}
