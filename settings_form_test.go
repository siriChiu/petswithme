package main

import (
	"reflect"
	"testing"
)

func validSettingsForm() SettingsForm {
	return SettingsForm{192, ActivityLively, true, "80", "40", "15", "12", "420"}
}
func TestSettingsFormApplyAndPreserveUntouchedFields(t *testing.T) {
	s := NormalizeSettings(Settings{Size: 144, ExperimentalMovement: true, CPU: DefaultCPUSettings()})
	got, e := ParseSettingsForm(s, validSettingsForm())
	if e != nil {
		t.Fatal(e)
	}
	if got.Size != 192 || got.Activity != ActivityLively || got.Quiet || !got.ExperimentalMovement || got.CPU.EnterPercent != 80 || got.CPU.ExitPercent != 40 || got.CPU.StretchEverySeconds != 420 {
		t.Fatalf("unexpected form:%+v", got)
	}
	f := validSettingsForm()
	f.Activity = ActivityQuiet
	f.CPUEnabled = false
	got, e = ParseSettingsForm(s, f)
	if e != nil || !got.Quiet || got.CPU.Enabled {
		t.Fatal("quiet/disable not applied")
	}
}
func TestSettingsFormRejectsWithoutMutation(t *testing.T) {
	base := NormalizeSettings(Settings{Size: 96, CPU: DefaultCPUSettings()})
	for _, bad := range []string{"", "NaN", "Inf", "-1", "0", "100", "cats"} {
		f := validSettingsForm()
		f.EnterPercent = bad
		got, e := ParseSettingsForm(base, f)
		if e == nil || !reflect.DeepEqual(got, base) {
			t.Fatalf("invalid percentage accepted:%q %+v", bad, got)
		}
	}
	for _, edit := range []func(*SettingsForm){func(f *SettingsForm) { f.Size = 123 }, func(f *SettingsForm) { f.Activity = "bogus" }, func(f *SettingsForm) { f.ExitPercent = "90" }, func(f *SettingsForm) { f.EnterSeconds = "0" }, func(f *SettingsForm) { f.ExitSeconds = "86401" }, func(f *SettingsForm) { f.StretchSeconds = "-5" }} {
		f := validSettingsForm()
		edit(&f)
		got, e := ParseSettingsForm(base, f)
		if e == nil || !reflect.DeepEqual(got, base) {
			t.Fatal("invalid form changed settings")
		}
	}
}
