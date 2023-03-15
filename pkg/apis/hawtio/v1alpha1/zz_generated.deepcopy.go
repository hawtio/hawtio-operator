//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Hawtio) DeepCopyInto(out *Hawtio) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Hawtio.
func (in *Hawtio) DeepCopy() *Hawtio {
	if in == nil {
		return nil
	}
	out := new(Hawtio)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Hawtio) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioAbout) DeepCopyInto(out *HawtioAbout) {
	*out = *in
	if in.ProductInfos != nil {
		in, out := &in.ProductInfos, &out.ProductInfos
		*out = make([]HawtioProductInfo, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioAbout.
func (in *HawtioAbout) DeepCopy() *HawtioAbout {
	if in == nil {
		return nil
	}
	out := new(HawtioAbout)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioAuth) DeepCopyInto(out *HawtioAuth) {
	*out = *in
	if in.ClientCertExpirationDate != nil {
		in, out := &in.ClientCertExpirationDate, &out.ClientCertExpirationDate
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioAuth.
func (in *HawtioAuth) DeepCopy() *HawtioAuth {
	if in == nil {
		return nil
	}
	out := new(HawtioAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioBranding) DeepCopyInto(out *HawtioBranding) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioBranding.
func (in *HawtioBranding) DeepCopy() *HawtioBranding {
	if in == nil {
		return nil
	}
	out := new(HawtioBranding)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioConfig) DeepCopyInto(out *HawtioConfig) {
	*out = *in
	in.About.DeepCopyInto(&out.About)
	out.Branding = in.Branding
	out.Online = in.Online
	if in.DisabledRoutes != nil {
		in, out := &in.DisabledRoutes, &out.DisabledRoutes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioConfig.
func (in *HawtioConfig) DeepCopy() *HawtioConfig {
	if in == nil {
		return nil
	}
	out := new(HawtioConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioConsoleLink) DeepCopyInto(out *HawtioConsoleLink) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioConsoleLink.
func (in *HawtioConsoleLink) DeepCopy() *HawtioConsoleLink {
	if in == nil {
		return nil
	}
	out := new(HawtioConsoleLink)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioList) DeepCopyInto(out *HawtioList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Hawtio, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioList.
func (in *HawtioList) DeepCopy() *HawtioList {
	if in == nil {
		return nil
	}
	out := new(HawtioList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HawtioList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioMetadataPropagation) DeepCopyInto(out *HawtioMetadataPropagation) {
	*out = *in
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioMetadataPropagation.
func (in *HawtioMetadataPropagation) DeepCopy() *HawtioMetadataPropagation {
	if in == nil {
		return nil
	}
	out := new(HawtioMetadataPropagation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioNginx) DeepCopyInto(out *HawtioNginx) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioNginx.
func (in *HawtioNginx) DeepCopy() *HawtioNginx {
	if in == nil {
		return nil
	}
	out := new(HawtioNginx)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioOnline) DeepCopyInto(out *HawtioOnline) {
	*out = *in
	out.ConsoleLink = in.ConsoleLink
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioOnline.
func (in *HawtioOnline) DeepCopy() *HawtioOnline {
	if in == nil {
		return nil
	}
	out := new(HawtioOnline)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioProductInfo) DeepCopyInto(out *HawtioProductInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioProductInfo.
func (in *HawtioProductInfo) DeepCopy() *HawtioProductInfo {
	if in == nil {
		return nil
	}
	out := new(HawtioProductInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioRBAC) DeepCopyInto(out *HawtioRBAC) {
	*out = *in
	if in.DisableRBACRegistry != nil {
		in, out := &in.DisableRBACRegistry, &out.DisableRBACRegistry
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioRBAC.
func (in *HawtioRBAC) DeepCopy() *HawtioRBAC {
	if in == nil {
		return nil
	}
	out := new(HawtioRBAC)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioRoute) DeepCopyInto(out *HawtioRoute) {
	*out = *in
	out.CertSecret = in.CertSecret
	in.CaCert.DeepCopyInto(&out.CaCert)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioRoute.
func (in *HawtioRoute) DeepCopy() *HawtioRoute {
	if in == nil {
		return nil
	}
	out := new(HawtioRoute)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioSpec) DeepCopyInto(out *HawtioSpec) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.MetadataPropagation.DeepCopyInto(&out.MetadataPropagation)
	in.Route.DeepCopyInto(&out.Route)
	in.Auth.DeepCopyInto(&out.Auth)
	out.Nginx = in.Nginx
	in.RBAC.DeepCopyInto(&out.RBAC)
	in.Resources.DeepCopyInto(&out.Resources)
	in.Config.DeepCopyInto(&out.Config)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioSpec.
func (in *HawtioSpec) DeepCopy() *HawtioSpec {
	if in == nil {
		return nil
	}
	out := new(HawtioSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HawtioStatus) DeepCopyInto(out *HawtioStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HawtioStatus.
func (in *HawtioStatus) DeepCopy() *HawtioStatus {
	if in == nil {
		return nil
	}
	out := new(HawtioStatus)
	in.DeepCopyInto(out)
	return out
}
