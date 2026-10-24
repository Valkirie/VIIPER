# Managed C API contract

`viiper_managed_abi()` returns `1` for the following contract:

- `viiper_device_add` and `viiper_device_add_ex` export devices only. They never import them into a driver, run an executable, or silently report an attachment failure as success.
- The managed owner registers feedback, imports using USBIP 0.9.8.1+ IOCTLs, tracks the returned port, and detaches that port before removing the export. Never combine managed attachment with `viiper_device_attach`.
- `viiper_device_attach` remains an explicit IOCTL-only operation for other callers. It does not return ownership metadata to the managed host and is not used by HandheldCompanion.
- `viiper_init` returns an error when the listener cannot bind, instead of waiting indefinitely. Inspect `viiper_last_error` after failure.
- Serialize lifecycle/C API operations in the host. Feedback callbacks must not synchronously re-enter the C API.

Windows attachment requires USBIP 0.9.8.1 or newer and uses one-shot imports. There is no executable or older-driver fallback. This changes previous implicit-attachment behavior; consumers must opt into explicit attachment ownership.

Build from the repository root with an x64 CGo compiler available:

`go build -buildmode=c-shared -o libviiper.dll ./clib/`

Check the Windows IOCTL ABI without importing any device:

`go test ./internal/server/api -run TestUSBIP981Layout -count=1`

HandheldCompanion also provides `scripts/Check-Viiper.cs` to exercise export-only lifecycle, occupied-port failure/recovery, and device wire formats against the built DLL without attaching devices.
