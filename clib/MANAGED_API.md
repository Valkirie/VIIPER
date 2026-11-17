# Managed C API contract

`viiper_managed_abi()` returns `1` for the following contract:

- `viiper_device_add` and `viiper_device_add_ex` export devices only. They never import them into a driver, run an executable, or silently report an attachment failure as success.
- The managed owner registers feedback, imports using USBIP 0.9.8.1+ IOCTLs, tracks the returned port, and reconciles imports by endpoint and export identity before removing the export. Never combine managed attachment with `viiper_device_attach`.
- Each exported device accepts only one active USB/IP importer. Competing imports receive `ST_DEV_BUSY`; disconnect, failed handshake, device removal, and shutdown release ownership. A reused textual bus/device ID is a new device identity.
- `viiper_device_attach` remains an explicit IOCTL-only operation for other callers. It does not return ownership metadata to the managed host and is not used by HandheldCompanion.
- `viiper_init` returns an error when the listener cannot bind, instead of waiting indefinitely. Inspect `viiper_last_error` after failure.
- Serialize lifecycle/C API operations in the host. Feedback callbacks must not synchronously re-enter the C API.

Windows attachment requires USBIP 0.9.8.1 or newer and uses one-shot imports. There is no executable or older-driver fallback. This changes previous implicit-attachment behavior; consumers must opt into explicit attachment ownership.

`PLUGIN_HARDWARE_ONCE` disables initial attach retries only. Socket loss after a successful connection can still queue kernel reconnect attempts, which can survive the host process. After successfully binding its exclusive listener, the managed host must reconcile stale imports matching its loopback endpoint. Before reusing or removing an export, cancel reconnect attempts for that exact host/service/busid and detach matching imports. Never issue global cancellation or detach-all for this purpose. Preserve cleanup ownership when an operation fails; the native exclusivity guard is still necessary because reconnect and new-import requests can race.

Build from the repository root with an x64 CGo compiler available:

`go build -buildmode=c-shared -o libviiper.dll ./clib/`

Check the Windows IOCTL ABI without importing any device:

`go test ./internal/server/api -run TestUSBIP981Layout -count=1`

Check exclusive-import lifetimes using in-memory connections only:

`go test -race ./internal/server/usb -run TestImportOwnership -count=1`

HandheldCompanion also provides `scripts/Check-Viiper.cs` to exercise export-only lifecycle, occupied-port failure/recovery, and device wire formats against the built DLL without attaching devices.
