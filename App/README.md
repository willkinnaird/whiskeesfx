# WhiskeesFX app

A small Windows tray app for the VisualFX module: Basic and Advanced settings, automatic aircraft profiles,
test switches, live status, and automatic updates from GitHub Releases.

## How it runs

- **One background program, tiny and GPU-free.** `WhiskeesFX.exe` is a single Go program (about 13 MB of memory).
  It serves its settings page on `127.0.0.1:47821` and has a tray icon. It draws nothing and never touches
  the graphics card.
- **The window is opened on demand.** It's Edge in app mode with its own profile and `--disable-gpu`, so it's drawn
  by the CPU and takes no graphics memory, even while it's open. "Hide to tray", or closing it, shuts that window's
  processes completely, so they hold no memory at all.
- **While you fly** it checks file times every 2 s and reads a file only when it has changed. Twice a minute it
  checks start-with-Windows and the installed package versions, and every 6 hours it checks for updates.
- **The page itself** polls once a second, stops when the window is minimised, attaches its event listeners once,
  and only rebuilds the parts of the page whose data changed.

## Where things live

| What | Where |
|---|---|
| App settings, your tweaks | `%APPDATA%\WhiskeesFX\config.json` |
| Downloaded defaults + shipped profiles | `%APPDATA%\WhiskeesFX\defaults\` (replaced by updates) |
| Your own aircraft profiles | `%APPDATA%\WhiskeesFX\myprofiles\` (never touched by updates) |
| The settings window's Edge profile, downloads | `%LOCALAPPDATA%\WhiskeesFX\` |
| Live settings the module reads | `...\WASM\MSFS2024\whiskeesfx-visualfx\work\visualfx.ini` |

## The live file is always built the same way

`defaults` → `aircraft profile` → `your tweaks (all aircraft, then this aircraft)` → `Basic-view group switches and
strength` → `test switches`

- Updates replace the defaults, but your tweaks are applied on top, so they survive.
- Strength multiplies each effect's `gm_alpha`. It's skipped for any effect where adding the dial would take it over
  16, because the sim ignores all dials above that.
- Positions (`engine_*`, `main_gear_*`, `nose_gear_*`, `ground_y`, `var*`) changed in the app are saved for the
  current aircraft only.
- If someone edits the live file by hand, the app stops writing it and shows a Resume button.

## Developer mode (Will's PC)

On first run, if `C:\MSFS 2024 SDK\WhiskeesFX\VisualFX\visualfx.ini` exists, it becomes the source of the defaults.
Editing that master copy is picked up within 2 s, so live tuning works exactly as before. "Write my tweaks into the
defaults file" bakes your app tweaks into the master copy.

## Aircraft profiles

These are in `profiles/*.ini`, and the app picks one from the loaded aircraft's title (the longest `match=` wins).
Without a match it uses `generic.ini`, which reads the sim's standard N1 instead of the Fenix's own. "New profile
for this aircraft" saves the current positions under the aircraft's title.

## Releasing

1. Build the two Community packages with the SDK as usual.
2. Run `PUBLISH-RELEASE.bat 1.2.0 "What changed"`. It copies the packages into `packages\`, commits, tags and pushes.
3. GitHub Actions (`.github/workflows/release.yml`) builds `WhiskeesFX.exe` and publishes the release with
   `defaults.zip` and the package zips.
4. Installed apps pick it up on their next check:
   - Settings and profiles update straight away.
   - The packages update as soon as the sim is closed.
   - The app updates itself and restarts.

## Building by hand

```
cd App
cp ../VisualFX/visualfx.ini defaults/visualfx.ini
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=v0.1.0 -X main.defaultRepo=OWNER/REPO" -o WhiskeesFX.exe .
```

For testing on Linux, `go build` gives a version with no tray. Point `WFX_HOME`, `WFX_WORK` and `WFX_COMMUNITY`
at test folders.
