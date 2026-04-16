# Android Termux Support Design

## Goal

Add an officially documented Android usage path for `chatbox` without turning the project into a native Android app.

The target is a CLI workflow running inside Termux on modern Android ARM64 devices.

## Product Decision

- Support target: `android/arm64`
- Runtime model: Termux command-line usage
- Non-goal: APK, foreground service, native Android UI, push notifications
- Keep protocol, transcript format, and chat flows unchanged

## Why This Scope

The existing app is already a Go CLI and cross-compiles cleanly to Android ARM64. The shortest path to useful Android support is not building an app shell, but making distribution, update behavior, and docs reflect the Android CLI reality.

This keeps the work small and reversible:

- no protocol changes
- no mobile-specific UI layer
- no new persistence model
- no Android SDK or JNI dependency

## Distribution Design

GitHub releases should publish one additional archive:

- `chatbox_android_arm64.tar.gz`

This aligns Android distribution with the current release asset model and keeps checksum verification unchanged.

Manual release tooling and GitHub Actions should both produce the new asset.

## Update Design

Android should not use the existing in-place `self-update` flow.

Reasons:

- current updater only recognizes macOS assets
- Termux install paths and permissions are not guaranteed to support safe in-place replacement
- a wrong partial implementation would be worse than a clear manual-upgrade instruction

Desired behavior:

- startup version checks may still report that a newer release exists
- `chatbox self-update` on Android returns a clear error directing users to GitHub Releases for manual replacement

## Documentation Design

README should explicitly document:

- Android support is via Termux, not APK
- how to download or build an Android binary
- how to unpack and run it in Termux
- how to host and join from Android
- Android-specific limitations:
  - no macOS Terminal alert integration
  - no `self-update`
  - hosting over cellular often fails due to NAT/operator restrictions

## Testing

Add focused tests for:

- release asset selection includes `android/arm64`
- self-update rejects Android with a clear message
- release/manual asset lists include the Android archive

No protocol or integration test changes are needed because runtime chat behavior is unchanged.
