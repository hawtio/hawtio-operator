# Note
This is config that gets injected into the configmap that Hawto-Online uses to
brand the application. The operator looks for this file so it is mandatory to
include it so therefore its content must match the `hawtconfig.json` specified
in `../hawtio/branding/` (gitlab midstream repository). Changes must be synced
else the resulting configmap will overwrite with old values.
