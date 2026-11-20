package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	Enabled       bool `json:"enabled"`
	OnlyInCockpit bool `json:"onlyInCockpit"`
	InvertPitch   bool `json:"invertPitch"`

	Master           float64 `json:"master"`
	GroundBob        float64 `json:"groundBob"`
	Rumble           float64 `json:"rumble"`
	Bumps            float64 `json:"bumps"`
	TouchdownThump   float64 `json:"touchdownThump"`
	BrakeLurch       float64 `json:"brakeLurch"`
	AccelPush        float64 `json:"accelPush"`
	SpeedbrakeLurch  float64 `json:"speedbrakeLurch"`
	SpeedbrakeBuffet float64 `json:"speedbrakeBuffet"`
	Wheelbase        float64 `json:"wheelbase"` // metres, nose gear to main gear

	// Real head tracking passed through from Tobii Game Hub / opentrack (UDP).
	HeadEnabled bool    `json:"headEnabled"`
	HeadPort    int     `json:"headPort"`
	HeadRotGain float64 `json:"headRotGain"`
	HeadMovGain float64 `json:"headMovGain"`
	HeadHotkey  string  `json:"headHotkey"`
	JoyDevice   int     `json:"joyDevice"`
	JoyButton   int     `json:"joyButton"`

	// MSFS 2024 Tobii settings
	TobiiRatio      float64 `json:"tobiiRatio"`
	TobiiEyeResp    float64 `json:"tobiiEyeResp"`
	TobiiSensYaw    float64 `json:"tobiiSensYaw"`
	TobiiSensPitch  float64 `json:"tobiiSensPitch"`
	TobiiSensRoll2  float64 `json:"tobiiSensRoll2"`
	TobiiSensX      float64 `json:"tobiiSensX"`
	TobiiSensY      float64 `json:"tobiiSensY"`
	TobiiSensZ      float64 `json:"tobiiSensZ"`
	TobiiLimX       float64 `json:"tobiiLimX"`
	TobiiLimY       float64 `json:"tobiiLimY"`
	TobiiLimZ       float64 `json:"tobiiLimZ"`
	TobiiCenterStab float64 `json:"tobiiCenterStab"`
	TobiiAutoCenter bool    `json:"tobiiAutoCenter"`
	TobiiBoost      float64 `json:"tobiiBoost"`
	TobiiSmoothing  float64 `json:"tobiiSmoothing"`
	TobiiLimYaw     float64 `json:"tobiiLimYaw"`
	TobiiLimPitch   float64 `json:"tobiiLimPitch"`
	TobiiLimRoll    float64 `json:"tobiiLimRoll"`

	RecenterHotkey string `json:"recenterHotkey"`
	JoyRecDevice   int    `json:"joyRecDevice"`
	JoyRecButton   int    `json:"joyRecButton"`
}

func DefaultSettings() Settings {
	return Settings{
		Enabled: true, OnlyInCockpit: true,
		Master: 1, GroundBob: 1, Rumble: 1, Bumps: 1, TouchdownThump: 1,
		BrakeLurch: 1, AccelPush: 0.5, SpeedbrakeLurch: 1, SpeedbrakeBuffet: 1,
		Wheelbase:   10,
		HeadEnabled: true, HeadPort: 4242, HeadRotGain: 1, HeadMovGain: 1,
		HeadHotkey: "Ctrl+Shift+H", JoyDevice: -1, JoyButton: -1,
		RecenterHotkey: "Ctrl+Shift+R", JoyRecDevice: -1, JoyRecButton: -1,
		TobiiRatio: 1, TobiiEyeResp: 0, TobiiSensYaw: 0.09, TobiiSensPitch: 0.09, TobiiSensRoll2: 0.09,
		TobiiSensX: 0.09, TobiiSensY: 0.09, TobiiSensZ: 0.09, TobiiLimX: 300, TobiiLimY: 300, TobiiLimZ: 300,
		TobiiCenterStab: 0.62, TobiiAutoCenter: false, TobiiBoost: 0, TobiiSmoothing: 0.7,
		TobiiLimYaw: 160, TobiiLimPitch: 75, TobiiLimRoll: 30,
	}
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	p := filepath.Join(dir, "WhiskeesFX", "CameraFX", "settings.json")
	if _, err := os.Stat(p); os.IsNotExist(err) { // first run after the rename: bring GroundFeel's settings across
		if b, err := os.ReadFile(filepath.Join(dir, "GroundFeel", "settings.json")); err == nil {
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = os.WriteFile(p, b, 0o644)
		}
	}
	return p
}

func LoadSettings() Settings {
	s := DefaultSettings()
	b, err := os.ReadFile(settingsPath())
	if err == nil {
		_ = json.Unmarshal(b, &s)
		var raw map[string]any
		_ = json.Unmarshal(b, &raw)
		if _, ok := raw["tobiiSensYaw"]; !ok { // saved by an older version: start from the MSFS-matching defaults
			d := DefaultSettings()
			s.TobiiRatio, s.TobiiEyeResp, s.TobiiSensYaw, s.TobiiSensPitch, s.TobiiSensRoll2 = d.TobiiRatio, d.TobiiEyeResp, d.TobiiSensYaw, d.TobiiSensPitch, d.TobiiSensRoll2
			s.TobiiSensX, s.TobiiSensY, s.TobiiSensZ, s.TobiiLimX, s.TobiiLimY, s.TobiiLimZ = d.TobiiSensX, d.TobiiSensY, d.TobiiSensZ, d.TobiiLimX, d.TobiiLimY, d.TobiiLimZ
			s.TobiiCenterStab, s.TobiiAutoCenter, s.TobiiBoost = d.TobiiCenterStab, d.TobiiAutoCenter, d.TobiiBoost
		}
	}
	s.sanitize()
	return s
}

func (s *Settings) sanitize() {
	c := func(v *float64, lo, hi float64) { *v = clamp(*v, lo, hi) }
	c(&s.Master, 0, 3)
	for _, v := range []*float64{&s.GroundBob, &s.Rumble, &s.Bumps, &s.TouchdownThump,
		&s.BrakeLurch, &s.AccelPush, &s.SpeedbrakeLurch, &s.SpeedbrakeBuffet} {
		c(v, 0, 3)
	}
	c(&s.Wheelbase, 0.5, 40)
	c(&s.HeadRotGain, 0, 4)
	for _, v := range []*float64{&s.TobiiRatio, &s.TobiiEyeResp, &s.TobiiSensYaw, &s.TobiiSensPitch, &s.TobiiSensRoll2,
		&s.TobiiSensX, &s.TobiiSensY, &s.TobiiSensZ, &s.TobiiCenterStab, &s.TobiiBoost, &s.TobiiSmoothing} {
		c(v, 0, 1)
	}
	for _, v := range []*float64{&s.TobiiLimX, &s.TobiiLimY, &s.TobiiLimZ} {
		c(v, 0, 300)
	}
	c(&s.TobiiLimYaw, 0, 180)
	c(&s.TobiiLimPitch, 0, 90)
	c(&s.TobiiLimRoll, 0, 90)
	if s.HeadPort < 1024 || s.HeadPort > 65535 {
		s.HeadPort = 4242
	}
	if s.JoyDevice < -1 || s.JoyDevice > 15 || s.JoyButton < -1 || s.JoyButton > 31 {
		s.JoyDevice, s.JoyButton = -1, -1
	}
	if s.JoyRecDevice < -1 || s.JoyRecDevice > 15 || s.JoyRecButton < -1 || s.JoyRecButton > 31 {
		s.JoyRecDevice, s.JoyRecButton = -1, -1
	}
}

func (s Settings) tobiiParams() TobiiParams {
	return TobiiParams{s.TobiiRatio, s.TobiiEyeResp, s.TobiiSensYaw, s.TobiiSensPitch, s.TobiiSensRoll2,
		s.TobiiSensX, s.TobiiSensY, s.TobiiSensZ, s.TobiiLimX, s.TobiiLimY, s.TobiiLimZ,
		s.TobiiCenterStab, s.TobiiAutoCenter, s.TobiiBoost, s.TobiiSmoothing,
		s.TobiiLimYaw, s.TobiiLimPitch, s.TobiiLimRoll}
}

func SaveSettings(s Settings) error {
	p := settingsPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(p, b, 0o644)
}
