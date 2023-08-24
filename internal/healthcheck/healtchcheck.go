package healthcheck

// Port is the port for the healthcheck sidecar.
const Port = 1251

// Mount is the mount point for the healthcheck sidecar pvc.
// it should be mounted on /<Mount>/<pvc>
const Mount = "/mnt"
