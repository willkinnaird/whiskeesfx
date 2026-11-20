WhiskeesFX for MSFS 2024
========================
CameraFX  (formerly GroundFeel)        - runway bumps, oleo bob, braking lurch and Tobii head tracking. Windows app: CameraFX\CameraFX.exe
VisualFX  (formerly GroundFeel RainFX) - rain, snow, reverse spray, vapour, condensation, vortices, tyre smoke. MSFS WASM module + custom effects.

VisualFX
  VisualFX\BUILD-VISUALFX.bat          builds the module and installs it into Community\whiskeesfx-visualfx
  VisualFX\effects\WhiskeesFX_VisualFX.xml   Dev Mode project for the custom effects (package whiskeesfx-visualfx-effects)
  VisualFX\visualfx.ini                 master copy of the settings
  Live settings:  %APPDATA%\Microsoft Flight Simulator 2024\WASM\MSFS2024\whiskeesfx-visualfx\work\visualfx.ini
  Live status:    ...\whiskeesfx-visualfx\work\status.txt
  VisualFX\tools\                       Python generators for the custom effect XML (make_wfx.py = all v12 effects)
  VisualFX\RESEARCH.md                  when each effect happens in real life, with sources
  VisualFX\visualfx_before_v12.ini      the last settings that used Asobo effects (fallback)

v12 (19 Nov 2026): every effect is a WhiskeesFX custom effect (WFX_Mist, WFX_Trail, WFX_Droplets, WFX_HeatHaze,
WingWater) with ~40 live dials each: speed, cone angles, gravity, drag, wind, corkscrew, size/growth, life, fade
shape, colour (grey, r/g/b, blue tint, glow), spin, rate and start bursts. The module (visualfx.wasm v12) takes up
to 256 effects, 48 dials per effect, a 512 KB settings file and [preset.NAME] sections.
If the new effects show "spawn FAILED" in status.txt, see the top of visualfx.ini (one-line fallback to the
"Safe" twins or the old GenericMist/GenericTrail).

Custom effects only register when the sim first loads the package: after changing them, Build All, copy the
built package into Community, and restart the sim.
